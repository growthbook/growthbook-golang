package growthbook

//------------------------------------------------------------------------------
//
//  LOGGING
//

var logger Logger

// SetLogger sets up the logging interface used throughout.
func SetLogger(userLogger Logger) {
	logger = userLogger
}

// Logger is a common interface for logging information and warning
// messages (errors are returned directly by SDK functions, but there
// is some useful "out of band" data that's provided via this
// interface).
type Logger interface {
	Warn(msg string, args ...interface{})
	Warnf(format string, args ...interface{})
	Info(msg string, args ...interface{})
	Infof(format string, args ...interface{})
}

// Internal logging functions wired up to this interface.

func warn(msg string, args ...interface{}) {
	if logger != nil {
		logger.Warn(msg, args...)
	}
}

func warnf(format string, args ...interface{}) {
	if logger != nil {
		logger.Warnf(format, args...)
	}
}

func info(msg string, args ...interface{}) {
	if logger != nil {
		logger.Info(msg, args...)
	}
}

func infof(format string, args ...interface{}) {
	if logger != nil {
		logger.Infof(format, args...)
	}
}

//------------------------------------------------------------------------------

// GrowthBook is the main export of the SDK.
type GrowthBook struct {
	Context *Context
}

// New created a new GrowthBook instance.
func New(context *Context) *GrowthBook {
	return &GrowthBook{context}
}

// Attributes returns the attributes from a GrowthBook's context.
func (gb *GrowthBook) Attributes() Attributes {
	return gb.Context.Attributes
}

// SetAttributes updates the attributes in a GrowthBook's context.
func (gb *GrowthBook) SetAttributes(attrs Attributes) {
	gb.Context.Attributes = attrs
}

// Features returns the features from a GrowthBook's context.
func (gb *GrowthBook) Features() FeatureMap {
	return gb.Context.Features
}

// SetFeatures update the features in a GrowthBook's context.
func (gb *GrowthBook) SetFeatures(features FeatureMap) {
	gb.Context.Features = features
}

// ForcedVariations returns the forced variations from a GrowthBook's
// context.
func (gb *GrowthBook) ForcedVariations() ForcedVariationsMap {
	return gb.Context.ForcedVariations
}

// URL returns the URL from a GrowthBook's context.
func (gb *GrowthBook) URL() *string {
	return gb.Context.URL
}

// Enabled returns the enabled flag from a GrowthBook's context.
func (gb *GrowthBook) Enabled() bool {
	return gb.Context.Enabled
}

func truthy(v interface{}) bool {
	if v == nil {
		return false
	}
	switch v.(type) {
	case string:
		return v.(string) != ""
	case bool:
		return v.(bool)
	case int:
		return v.(int) != 0
	case uint:
		return v.(uint) != 0
	case float32:
		return v.(float32) != 0
	case float64:
		return v.(float64) != 0
	}
	return true
}

func getFeatureResult(value interface{}, source FeatureResultSource,
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

// GetValueWithDefault ...
func (fr *FeatureResult) GetValueWithDefault(def interface{}) interface{} {
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
		// fmt.Println("Returning unknown feature")
		return getFeatureResult(nil, UnknownFeatureResultSource, nil, nil)
	}
	// fmt.Printf("  feature=%#v\n", *feature)

	// Loop through the feature rules (if any).
	for _, rule := range feature.Rules {
		// If the rule has a condition and the condition does not pass,
		// skip this rule.
		if rule.Condition != nil && !rule.Condition.Eval(gb.Attributes()) {
			info("Skip rule because of condition", key, rule)
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
					info("Skip rule because of missing hash attribute", key, rule)
					continue
				}
				hashString, ok := hashValue.(string)
				if !ok {
					warn("Skip rule because of non-string hash attribute", key, rule)
					continue
				}
				if hashString == "" {
					info("Skip rule because of empty hash attribute", key, rule)
					continue
				}

				// Hash the value.
				n := float64(hashFnv32a(hashString+key)%1000) / 1000

				// If the hash is greater than rule.Coverage, skip the rule.
				if n > *rule.Coverage {
					info("Skip rule because of coverage", key, rule)
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
		// fmt.Printf("Running experiment: %#v\n", experiment)
		result := gb.Run(&experiment)
		// fmt.Printf("  ==> %#v\n", result)

		// If result.inExperiment is false, skip this rule and continue.
		if !result.InExperiment {
			info("Skip rule because user not in experiment", key, rule)
			continue
		}

		// Otherwise, return experiment result.
		return getFeatureResult(result.Value, ExperimentResultSource, &experiment, result)
	}

	// Fall back to using the default value
	return getFeatureResult(feature.DefaultValue, DefaultValueResultSource, nil, nil)
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
	var value interface{}
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
	// 1. If experiment.variations has fewer than 2 variations, return
	//    getExperimentResult(experiment).
	// fmt.Printf("===> Run 1: len(variations) = %d\n", len(exp.Variations))
	if len(exp.Variations) < 2 {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 2. If context.enabled is false, return
	//    getExperimentResult(experiment)
	// fmt.Printf("===> Run 2: enabled = %t\n", gb.Context.Enabled)
	if !gb.Context.Enabled {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 3. If context.url exists
	//
	// qsOverride = getQueryStringOverride(experiment.key, context.url);
	// if (qsOverride != null) {
	//   return getExperimentResult(experiment, qsOverride);
	// }
	// fmt.Printf("===> Run 3: URL = %#v\n", gb.Context.URL)
	if gb.Context.URL != nil {
		qsOverride := getQueryStringOverride(exp.Key, *gb.Context.URL, len(exp.Variations))
		if qsOverride != nil {
			return gb.getExperimentResult(exp, *qsOverride, false)
		}
	}

	// 4. Return if forced via context
	//
	// if (experiment.key in context.forcedVariations) {
	//   return getExperimentResult(
	//     experiment,
	//     context.forcedVariations[experiment.key]
	//   );
	// }
	force, forced := gb.Context.ForcedVariations[exp.Key]
	// fmt.Printf("===> Run 4: forced = %t\n", forced)
	if forced {
		return gb.getExperimentResult(exp, force, false)
	}

	// 5. If experiment.active is set to false, return
	//    getExperimentResult(experiment)
	// fmt.Printf("===> Run 5: active = %t\n", exp.Active)
	if !exp.Active {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 6. Get the user hash value and return if empty
	//
	// hashAttribute = experiment.hashAttribute || "id";
	// hashValue = context.attributes[hashAttribute] || "";
	// if (hashValue == "") {
	//   return getExperimentResult(experiment);
	// }
	hashAttribute := "id"
	if exp.HashAttribute != nil {
		hashAttribute = *exp.HashAttribute
	}
	// fmt.Printf("===> Run 6: hashAttribute = %s\n", hashAttribute)
	hashValue := ""
	if _, ok := gb.Context.Attributes[hashAttribute]; ok {
		tmp, ok := gb.Context.Attributes[hashAttribute].(string)
		if ok {
			hashValue = tmp
		}
	}
	// fmt.Printf("===> Run 6: hashValue = %s\n", hashValue)
	if hashValue == "" {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 7. If experiment.namespace is set, return if not in range
	//
	// if (!inNamespace(hashValue, experiment.namespace)) {
	//   return getExperimentResult(experiment);
	// }
	// fmt.Printf("===> Run 7: namespace = %#v\n", exp.Namespace)
	if exp.Namespace != nil {
		if !inNamespace(hashValue, exp.Namespace) {
			return gb.getExperimentResult(exp, 0, false)
		}
	}

	// 8. If experiment.condition is set return if it evaluates to false
	//
	// if (!evalCondition(context.attributes, experiment.condition)) {
	//   return getExperimentResult(experiment);
	// }
	// fmt.Printf("===> Run 8: condition = %#v\n", exp.Condition)
	if exp.Condition != nil {
		if !exp.Condition.Eval(gb.Context.Attributes) {
			return gb.getExperimentResult(exp, 0, false)
		}
	}

	// 9. Calculate bucket ranges for the variations and choose one
	//
	// ranges = getBucketRanges(
	//   experiment.variations.length,
	//   experiment.converage ?? 1,
	//   experiment.weights ?? []
	// );
	// n = hash(hashValue + experiment.key);
	// assigned = chooseVariation(n, ranges);

	coverage := float64(1)
	if exp.Coverage != nil {
		coverage = *exp.Coverage
	}
	// fmt.Printf("===> Run 9: coverage = %v\n", coverage)
	ranges := getBucketRanges(len(exp.Variations), coverage, exp.Weights)
	// fmt.Printf("===> Run 9: ranges = %#v\n", ranges)
	n := float64(hashFnv32a(hashValue+exp.Key)%1000) / 1000
	// fmt.Printf("===> Run 9: n = %v\n", n)
	assigned := chooseVariation(float64(n), ranges)

	// 10. If assigned == -1, return getExperimentResult(experiment)
	// fmt.Printf("===> Run 10: assigned = %v\n", assigned)
	if assigned == -1 {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 11. If experiment has a forced variation, return
	//
	// if ("force" in experiment) {
	//   return getExperimentResult(experiment, experiment.force);
	// }
	// fmt.Printf("===> Run 11: force = %#v\n", exp.Force)
	if exp.Force != nil {
		return gb.getExperimentResult(exp, *exp.Force, false)
	}

	// 12. If context.qaMode, return getExperimentResult(experiment)
	// fmt.Printf("===> Run 12: qamode = %t\n", gb.Context.QaMode)
	if gb.Context.QaMode {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 13. Build the result object
	//
	result := gb.getExperimentResult(exp, assigned, true)
	// fmt.Printf("===> Run 13: result = %#v\n", result)

	// 14. Fire context.trackingCallback if set and the combination of
	//     hashAttribute, hashValue, experiment.key, and variationId has
	//     not been tracked before
	gb.track(exp, result)

	// 15. Return result
	return result
}

func (gb *GrowthBook) track(exp *Experiment, result *ExperimentResult) {
	// TODO: FILL THIS IN!
}
