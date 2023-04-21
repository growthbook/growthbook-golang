// Package growthbook provides a Go SDK for the GrowthBook A/B testing
// and feature flagging service.
package growthbook

import (
	"net/url"
	"strconv"
)

type subscriptionID uint

// GrowthBook is the main export of the SDK.
type GrowthBook struct {
	context            *Context
	trackedExperiments map[string]bool
	nextSubscriptionID subscriptionID
	subscriptions      map[subscriptionID]ExperimentCallback
	latestResults      map[string]*ExperimentResult
}

// New created a new GrowthBook instance.
func New(context *Context) *GrowthBook {
	return &GrowthBook{
		context,
		map[string]bool{},
		1, map[subscriptionID]ExperimentCallback{},
		map[string]*ExperimentResult{},
	}
}

// Attributes returns the attributes from a GrowthBook's context.
func (gb *GrowthBook) Attributes() Attributes {
	return gb.context.Attributes
}

// WithAttributes updates the attributes in a GrowthBook's context.
func (gb *GrowthBook) WithAttributes(attrs Attributes) *GrowthBook {
	gb.context.Attributes = attrs
	return gb
}

// Features returns the features from a GrowthBook's context.
func (gb *GrowthBook) Features() FeatureMap {
	return gb.context.Features
}

// WithFeatures update the features in a GrowthBook's context.
func (gb *GrowthBook) WithFeatures(features FeatureMap) *GrowthBook {
	gb.context.Features = features
	return gb
}

// ForcedVariations returns the forced variations from a GrowthBook's
// context.
func (gb *GrowthBook) ForcedVariations() ForcedVariationsMap {
	return gb.context.ForcedVariations
}

// WithForcedVariations sets the forced variations in a GrowthBook's
// context.
func (gb *GrowthBook) WithForcedVariations(forcedVariations ForcedVariationsMap) *GrowthBook {
	gb.context.ForcedVariations = forcedVariations
	return gb
}

// URL returns the URL from a GrowthBook's context.
func (gb *GrowthBook) URL() *url.URL {
	return gb.context.URL
}

// WithURL sets the URL in a GrowthBook's context.
func (gb *GrowthBook) WithURL(url *url.URL) *GrowthBook {
	gb.context.URL = url
	return gb
}

// Enabled returns the enabled flag from a GrowthBook's context.
func (gb *GrowthBook) Enabled() bool {
	return gb.context.Enabled
}

// WithEnabled sets the enabled flag in a GrowthBook's context.
func (gb *GrowthBook) WithEnabled(enabled bool) *GrowthBook {
	gb.context.Enabled = enabled
	return gb
}

// GetValueWithDefault extracts a value from a FeatureResult with a
// default.
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
	feature, ok := gb.context.Features[key]
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

		// Otherwise, convert the rule to an Experiment object, copying
		// values from the rule as necessary.
		experiment := Experiment{
			Key:        key,
			Variations: rule.Variations,
			Active:     true,
		}
		if rule.TrackingKey != nil {
			experiment.Key = *rule.TrackingKey
		}
		if rule.Coverage != nil {
			var tmp *float64
			if rule.Coverage != nil {
				val := *rule.Coverage
				tmp = &val
			}
			experiment.Coverage = tmp
		}
		if rule.Weights != nil {
			var tmp []float64
			if rule.Weights != nil {
				tmp = make([]float64, len(rule.Weights))
				copy(tmp, rule.Weights)
			}
			experiment.Weights = tmp
		}
		if rule.HashAttribute != nil {
			var tmp *string
			if rule.HashAttribute != nil {
				val := *rule.HashAttribute
				tmp = &val
			}
			experiment.HashAttribute = tmp
		}
		if rule.Namespace != nil {
			var tmp *Namespace
			if rule.Namespace != nil {
				val := Namespace{rule.Namespace.ID, rule.Namespace.Start, rule.Namespace.End}
				tmp = &val
			}
			experiment.Namespace = tmp
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
	if _, ok := gb.context.Attributes[hashAttribute]; ok {
		tmp, ok := gb.context.Attributes[hashAttribute].(string)
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

// Run an experiment. (Uses doRun to make wrapping for subscriptions
// simple.)
func (gb *GrowthBook) Run(exp *Experiment) *ExperimentResult {
	// Actually run the experiment.
	result := gb.doRun(exp)

	// Determine whether the result changed from the last stored result
	// for the experiment.
	changed := false
	storedResult, exists := gb.latestResults[exp.Key]
	if exists {
		if storedResult.InExperiment != result.InExperiment ||
			storedResult.VariationID != result.VariationID {
			changed = true
		}
	}

	// Store the experiment result.
	gb.latestResults[exp.Key] = result

	// If the result changed, trigger all subscriptions.
	if changed || !exists {
		for _, sub := range gb.subscriptions {
			sub(exp, result)
		}
	}

	return result
}

// Worker function to run an experiment.
func (gb *GrowthBook) doRun(exp *Experiment) *ExperimentResult {
	// 1. If exp.Variations has fewer than 2 variations, return default
	//    result.
	if len(exp.Variations) < 2 {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 2. If context.Enabled is false, return default result.
	if !gb.context.Enabled {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 3. If context.URL exists, check for query string override and use
	//    it if it exists.
	if gb.context.URL != nil {
		qsOverride := getQueryStringOverride(exp.Key, gb.context.URL, len(exp.Variations))
		if qsOverride != nil {
			return gb.getExperimentResult(exp, *qsOverride, false)
		}
	}

	// 4. Return forced result if forced via context.
	force, forced := gb.context.ForcedVariations[exp.Key]
	if forced {
		return gb.getExperimentResult(exp, force, false)
	}

	// 5. If exp.Active is set to false, return default result.
	if !exp.Active {
		return gb.getExperimentResult(exp, 0, false)
	}

	// 6. Get the user hash value and return if empty.
	hashAttribute := "id"
	if exp.HashAttribute != nil {
		hashAttribute = *exp.HashAttribute
	}
	hashValue := ""
	if _, ok := gb.context.Attributes[hashAttribute]; ok {
		tmp, ok := gb.context.Attributes[hashAttribute].(string)
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
		if !exp.Condition.Eval(gb.context.Attributes) {
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
	if gb.context.QAMode {
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
	if gb.context.TrackingCallback == nil {
		return
	}

	// Make sure tracking callback is only fired once per unique
	// experiment.
	key := result.HashAttribute + result.HashValue +
		exp.Key + strconv.Itoa(result.VariationID)
	if _, exists := gb.trackedExperiments[key]; exists {
		return
	}

	gb.trackedExperiments[key] = true
	gb.context.TrackingCallback(exp, result)
}

// Subscribe adds a callback that is called every time GrowthBook.Run
// is called. This is different from the tracking callback since it
// also fires when a user is not included in an experiment.
func (gb *GrowthBook) Subscribe(callback ExperimentCallback) func() {
	id := gb.nextSubscriptionID
	gb.subscriptions[id] = callback
	gb.nextSubscriptionID++
	return func() {
		delete(gb.subscriptions, id)
	}
}

// GetAllResults returns a map containing all the latest results from
// all experiments that have been run, indexed by the experiment key.
func (gb *GrowthBook) GetAllResults() map[string]*ExperimentResult {
	return gb.latestResults
}

// ClearSavedResults clears out any experiment results saved within a
// GrowthBook instance (used for deciding whether to send data to
// subscriptions).
func (gb *GrowthBook) ClearSavedResults() {
	gb.latestResults = map[string]*ExperimentResult{}
}

// ClearTrackingData clears out records of calls to the experiment
// tracking callback.
func (gb *GrowthBook) ClearTrackingData() {
	gb.trackedExperiments = map[string]bool{}
}
