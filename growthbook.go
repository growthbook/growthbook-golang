package growthbook

import (
	"net/url"
	"strconv"
)

// GrowthBook is the main export of the SDK.
type GrowthBook struct {
	Context            *Context
	TrackedExperiments map[string]bool
}

// New created a new GrowthBook instance.
func New(context *Context) *GrowthBook {
	return &GrowthBook{context, map[string]bool{}}
}

// Attributes returns the attributes from a GrowthBook's context.
func (gb *GrowthBook) Attributes() Attributes {
	return gb.Context.Attributes
}

// WithAttributes updates the attributes in a GrowthBook's context.
func (gb *GrowthBook) WithAttributes(attrs Attributes) *GrowthBook {
	gb.Context.Attributes = attrs
	return gb
}

// Features returns the features from a GrowthBook's context.
func (gb *GrowthBook) Features() FeatureMap {
	return gb.Context.Features
}

// WithFeatures update the features in a GrowthBook's context.
func (gb *GrowthBook) WithFeatures(features FeatureMap) *GrowthBook {
	gb.Context.Features = features
	return gb
}

// ForcedVariations returns the forced variations from a GrowthBook's
// context.
func (gb *GrowthBook) ForcedVariations() ForcedVariationsMap {
	return gb.Context.ForcedVariations
}

// WithForcedVariations sets the forced variations in a GrowthBook's
// context.
func (gb *GrowthBook) WithForcedVariations(forcedVariations ForcedVariationsMap) *GrowthBook {
	gb.Context.ForcedVariations = forcedVariations
	return gb
}

// URL returns the URL from a GrowthBook's context.
func (gb *GrowthBook) URL() *url.URL {
	return gb.Context.URL
}

// WithURL sets the URL in a GrowthBook's context.
func (gb *GrowthBook) WithURL(url *url.URL) *GrowthBook {
	gb.Context.URL = url
	return gb
}

// Enabled returns the enabled flag from a GrowthBook's context.
func (gb *GrowthBook) Enabled() bool {
	return gb.Context.Enabled
}

// WithEnabled sets the enabled flag in a GrowthBook's context.
func (gb *GrowthBook) WithEnabled(enabled bool) *GrowthBook {
	gb.Context.Enabled = enabled
	return gb
}

// GetValueWithDefault ...
func (fr *FeatureResult) GetValueWithDefault(def FeatureValue) FeatureValue {
	if fr.Value == nil {
		return def
	}
	return fr.Value
}

// Feature returns the result for a feature identified by a string
// feature key.
func (gb *GrowthBook) Feature(key string) *FeatureResult {
	// Handle unknown features.
	feature, ok := gb.Context.Features[key]
	if !ok {
		return getFeatureResult(nil, UnknownFeatureResultSource, nil, nil)
	}

	// Loop through the feature rules (if any).
	for _, rule := range feature.Rules {
		// If the rule has a condition and the condition does not pass,
		// skip this rule.
		if rule.Condition != nil && !rule.Condition.Eval(gb.Attributes()) {
			logInfo(InfoRuleSkipCondition, key, rule)
			continue
		}

		// If rule.Force has been set:
		if rule.Force != nil {
			// If rule.Coverage is set
			if rule.Coverage != nil {
				// Get the value of the hashAttribute, defaulting to "id", and
				// if missing or empty, skip the rule.
				hashAttribute := "id"
				if rule.HashAttribute != nil {
					hashAttribute = *rule.HashAttribute
				}
				hashValue, ok := gb.Attributes()[hashAttribute]
				if !ok {
					logInfo(InfoRuleSkipNoHashAttribute, key, rule)
					continue
				}
				hashString, ok := hashValue.(string)
				if !ok {
					logWarn(WarnRuleSkipHashAttributeType, key, rule)
					continue
				}
				if hashString == "" {
					logInfo(InfoRuleSkipEmptyHashAttribute, key, rule)
					continue
				}

				// Hash the value.
				n := float64(hashFnv32a(hashString+key)%1000) / 1000

				// If the hash is greater than rule.Coverage, skip the rule.
				if n > *rule.Coverage {
					logInfo(InfoRuleSkipCoverage, key, rule)
					continue
				}
			}

			// Return forced feature result.
			return getFeatureResult(rule.Force, ForceResultSource, nil, nil)
		}

		// Otherwise, convert the rule to an Experiment object.
		experiment := Experiment{
			Key:        key,
			Variations: rule.Variations,
			Active:     true,
		}
		if rule.TrackingKey != nil {
			experiment.Key = *rule.TrackingKey
		}
		if rule.Coverage != nil {
			// TODO: COPY?
			experiment.Coverage = rule.Coverage
		}
		if rule.Weights != nil {
			// TODO: COPY?
			experiment.Weights = rule.Weights
		}
		if rule.HashAttribute != nil {
			// TODO: COPY?
			experiment.HashAttribute = rule.HashAttribute
		}
		if rule.Namespace != nil {
			// TODO: COPY?
			experiment.Namespace = rule.Namespace
		}

		// Run the experiment.
		result := gb.Run(&experiment)

		// If result.inExperiment is false, skip this rule and continue.
		if !result.InExperiment {
			logInfo(InfoRuleSkipUserNotInExp, key, rule)
			continue
		}

		// Otherwise, return experiment result.
		return getFeatureResult(result.Value, ExperimentResultSource, &experiment, result)
	}

	// Fall back to using the default value
	return getFeatureResult(feature.DefaultValue, DefaultValueResultSource, nil, nil)
}

func getFeatureResult(value FeatureValue, source FeatureResultSource,
	experiment *Experiment, experimentResult *ExperimentResult) *FeatureResult {
	on := truthy(value)
	off := !on
	return &FeatureResult{
		Value:            value,
		On:               on,
		Off:              off,
		Source:           source,
		Experiment:       experiment,
		ExperimentResult: experimentResult,
	}
}

func (gb *GrowthBook) getExperimentResult(exp *Experiment, variationIndex int, inExperiment bool) *ExperimentResult {
	// Make sure the variationIndex is valid for the experiment
	if variationIndex < 0 || variationIndex >= len(exp.Variations) {
		variationIndex = 0
	}

	// Get the hashAttribute and hashValue
	hashAttribute := "id"
	if exp.HashAttribute != nil {
		hashAttribute = *exp.HashAttribute
	}
	hashValue := ""
	if _, ok := gb.Context.Attributes[hashAttribute]; ok {
		tmp, ok := gb.Context.Attributes[hashAttribute].(string)
		if ok {
			hashValue = tmp
		}
	}

	// Return
	var value FeatureValue
	if variationIndex < len(exp.Variations) {
		value = exp.Variations[variationIndex]
	}
	return &ExperimentResult{
		InExperiment:  inExperiment,
		VariationID:   variationIndex,
		Value:         value,
		HashAttribute: hashAttribute,
		HashValue:     hashValue,
	}
}

// Run ...
func (gb *GrowthBook) Run(exp *Experiment) *ExperimentResult {
	// 1. If exp.Variations has fewer than 2 variations, return default
	//    result.
	if len(exp.Variations) < 2 {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 2. If context.Enabled is false, return default result.
	if !gb.Context.Enabled {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 3. If context.URL exists, check for query string override and use
	//    it if it exists.
	if gb.Context.URL != nil {
		qsOverride := getQueryStringOverride(exp.Key, gb.Context.URL, len(exp.Variations))
		if qsOverride != nil {
			return gb.getExperimentResult(exp, *qsOverride, false)
		}
	}

	// 4. Return forced result if forced via context.
	force, forced := gb.Context.ForcedVariations[exp.Key]
	if forced {
		return gb.getExperimentResult(exp, force, false)
	}

	// 5. If exp.Active is set to false, return default result.
	// TODO: CHECK THAT THIS BEHAVIOUR IS RIGHT! Active MIGHT BE
	// OPTIONAL HERE -- IT SHOULD DEFAULT TO TRUE IF IT's NOT EXPLICITLY
	// SET TO FALSE!
	if !exp.Active {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 6. Get the user hash value and return if empty.
	hashAttribute := "id"
	if exp.HashAttribute != nil {
		hashAttribute = *exp.HashAttribute
	}
	hashValue := ""
	if _, ok := gb.Context.Attributes[hashAttribute]; ok {
		tmp, ok := gb.Context.Attributes[hashAttribute].(string)
		if ok {
			hashValue = tmp
		}
	}
	if hashValue == "" {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 7. If exp.Namespace is set, return if not in range.
	if exp.Namespace != nil {
		if !inNamespace(hashValue, exp.Namespace) {
			return gb.getExperimentResult(exp, 0, false)
		}
	}

	// 8. If exp.Condition is set, return if it evaluates to false.
	if exp.Condition != nil {
		if !exp.Condition.Eval(gb.Context.Attributes) {
			return gb.getExperimentResult(exp, 0, false)
		}
	}

	// 9. Calculate bucket ranges for the variations and choose one.
	coverage := float64(1)
	if exp.Coverage != nil {
		coverage = *exp.Coverage
	}
	ranges := getBucketRanges(len(exp.Variations), coverage, exp.Weights)
	n := float64(hashFnv32a(hashValue+exp.Key)%1000) / 1000
	assigned := chooseVariation(float64(n), ranges)

	// 10. If assigned == -1, return default result.
	if assigned == -1 {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 11. If experiment has a forced variation, return it.
	if exp.Force != nil {
		return gb.getExperimentResult(exp, *exp.Force, false)
	}

	// 12. If in QA mode, return default result.
	if gb.Context.QAMode {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 13. Build the result object.
	result := gb.getExperimentResult(exp, assigned, true)

	// 14. Fire tracking callback if required.
	gb.track(exp, result)

	return result
}

// Fire Context.TrackingCallback if it's set and the combination of
// hashAttribute, hashValue, experiment key, and variation ID has not
// been tracked before.
func (gb *GrowthBook) track(exp *Experiment, result *ExperimentResult) {
	if gb.Context.TrackingCallback == nil {
		return
	}

	// Make sure tracking callback is only fired once per unique
	// experiment.
	key := result.HashAttribute + result.HashValue +
		exp.Key + strconv.Itoa(result.VariationID)
	if _, exists := gb.TrackedExperiments[key]; exists {
		return
	}

	gb.TrackedExperiments[key] = true
	gb.Context.TrackingCallback(exp, result)
}
