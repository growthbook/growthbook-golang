// Package growthbook provides a Go SDK for the GrowthBook A/B testing
// and feature flagging service.
package growthbook

import (
	"fmt"
	"net/url"
	"strconv"
)

type subscriptionID uint

// GrowthBook is the main export of the SDK.
type GrowthBook struct {
	context            *Context
	attributeOverrides Attributes
	trackedExperiments map[string]bool
	nextSubscriptionID subscriptionID
	subscriptions      map[subscriptionID]ExperimentCallback
	latestResults      map[string]*Result
}

// New created a new GrowthBook instance.
func New(context *Context) *GrowthBook {
	if context == nil {
		context = NewContext()
	}
	return &GrowthBook{
		context,
		Attributes{},
		map[string]bool{},
		1, map[subscriptionID]ExperimentCallback{},
		map[string]*Result{},
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

// AttributeOverrides returns the current attribute overrides.
func (gb *GrowthBook) AttributeOverrides() Attributes {
	return gb.attributeOverrides
}

// WithAttributeOverrides returns the current attribute overrides.
func (gb *GrowthBook) WithAttributeOverrides(overrides Attributes) *GrowthBook {
	gb.attributeOverrides = overrides
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

func (gb *GrowthBook) getHashAttribute(attr string) (string, string) {
	hashAttribute := "id"
	if attr != "" {
		hashAttribute = attr
	}

	hashValue, ok := gb.AttributeOverrides()[hashAttribute]
	if !ok {
		hashValue, ok = gb.Attributes()[hashAttribute]
		if !ok {
			return "", ""
		}
	}
	hashString, ok := convertHashValue(hashValue)
	if !ok {
		return "", ""
	}

	return hashAttribute, hashString
}

func (gb *GrowthBook) isIncludedInRollout(
	seed string,
	hashAttribute string,
	rng *Range,
	coverage *float64,
	hashVersion int,
) bool {
	if rng == nil && coverage == nil {
		return true
	}

	_, hashValue := gb.getHashAttribute(hashAttribute)
	if hashValue == "" {
		return false
	}

	hv := 1
	if hashVersion != 0 {
		hv = hashVersion
	}
	n := hash(seed, hashValue, hv)
	if n == nil {
		return false
	}

	if rng != nil {
		return rng.InRange(*n)
	}
	if coverage != nil {
		return *n <= *coverage
	}
	return true
}

// Feature returns the result for a feature identified by a string
// feature key.
func (gb *GrowthBook) Feature(key string) *FeatureResult {
	// TODO: HANDLE GLOBAL OVERRIDES

	// Handle unknown features.
	feature, ok := gb.context.Features[key]
	if !ok {
		return getFeatureResult(key, nil, UnknownFeatureResultSource, "", nil, nil)
	}

	// Loop through the feature rules (if any).
	for _, rule := range feature.Rules {

		// If the rule has a condition and the condition does not pass,
		// skip this rule.
		if rule.Condition != nil && !rule.Condition.Eval(gb.Attributes()) {
			logInfo(InfoRuleSkipCondition, key, rule)
			continue
		}

		// TODO: HANDLE FILTERING OUT

		// If rule.Force has been set:
		if rule.Force != nil {
			seed := key
			if rule.Seed != "" {
				seed = rule.Seed
			}
			if !gb.isIncludedInRollout(
				seed,
				rule.HashAttribute,
				rule.Range,
				rule.Coverage,
				rule.HashVersion,
			) {
				logInfo(InfoRuleSkipUserNotInRollout, key, rule)
				continue
			}

			// TODO: MORE LOGGING

			// Return forced feature result.
			return getFeatureResult(key, rule.Force, ForceResultSource, rule.ID, nil, nil)
		}

		// Otherwise, convert the rule to an Experiment object, copying
		// values from the rule as necessary.
		experiment := Experiment{
			Key:        key,
			Variations: rule.Variations,
			Active:     true,
		}
		if rule.Key != "" {
			experiment.Key = rule.Key
		}
		if rule.Coverage != nil {
			val := *rule.Coverage
			experiment.Coverage = &val
		}
		if rule.Weights != nil {
			tmp := make([]float64, len(rule.Weights))
			copy(tmp, rule.Weights)
			experiment.Weights = tmp
		}
		if rule.HashAttribute != "" {
			experiment.HashAttribute = rule.HashAttribute
		}
		if rule.Namespace != nil {
			val := Namespace{rule.Namespace.ID, rule.Namespace.Start, rule.Namespace.End}
			experiment.Namespace = &val
		}
		if rule.Meta != nil {
			experiment.Meta = rule.Meta
		}
		if rule.Ranges != nil {
			experiment.Ranges = rule.Ranges
		}
		if rule.Name != "" {
			experiment.Name = rule.Name
		}
		if rule.Phase != "" {
			experiment.Phase = rule.Phase
		}
		if rule.Seed != "" {
			experiment.Seed = rule.Seed
		}
		if rule.HashVersion != 0 {
			experiment.HashVersion = rule.HashVersion
		}
		// if rule.Filters != nil {
		// 	experiment.Filters = rule.Filters
		// }

		// Run the experiment.
		result := gb.doRun(&experiment, key)

		// Only return a value if the user is part of the experiment.
		if result.InExperiment /* && !res.Passthrough */ {
			return getFeatureResult(key, result.Value, ExperimentResultSource, rule.ID, &experiment, result)
		}
	}

	// Fall back to using the default value
	return getFeatureResult(key, feature.DefaultValue, DefaultValueResultSource, "", nil, nil)
}

func getFeatureResult(
	key string,
	value FeatureValue,
	source FeatureResultSource,
	ruleID string,
	experiment *Experiment,
	result *Result) *FeatureResult {
	on := truthy(value)
	off := !on
	retval := FeatureResult{
		Value:            value,
		On:               on,
		Off:              off,
		Source:           source,
		RuleID:           ruleID,
		Experiment:       experiment,
		ExperimentResult: result,
	}

	// TODO: TRACK FEATURE USAGE

	return &retval
}

func (gb *GrowthBook) getResult(
	exp *Experiment, variationIndex int,
	hashUsed bool, featureID string, bucket *float64) *Result {
	inExperiment := true

	// If assigned variation is not valid, use the baseline and mark the
	// user as not in the experiment
	if variationIndex < 0 || variationIndex >= len(exp.Variations) {
		variationIndex = 0
		inExperiment = false
	}

	// Get the hashAttribute and hashValue
	hashAttribute := "id"
	if exp.HashAttribute != "" {
		hashAttribute = exp.HashAttribute
	}
	hashString := ""
	hashValue, ok := gb.context.Attributes[hashAttribute]
	if ok {
		hashString, _ = convertHashValue(hashValue)
	}

	// TODO: COPY META INFO FROM EXPERIMENT HERE
	var metaKey *string

	// Return
	var value FeatureValue
	if variationIndex < len(exp.Variations) {
		value = exp.Variations[variationIndex]
	}
	key := fmt.Sprint(variationIndex)
	if metaKey != nil {
		key = *metaKey
	}
	return &Result{
		Key:           key,
		FeatureID:     featureID,
		InExperiment:  inExperiment,
		HashUsed:      hashUsed,
		VariationID:   variationIndex,
		Value:         value,
		HashAttribute: hashAttribute,
		HashValue:     hashString,
		Bucket:        bucket,
	}

	// if (meta.name) res.name = meta.name;
	// if (meta.passthrough) res.passthrough = meta.passthrough;
}

// Run an experiment. (Uses doRun to make wrapping for subscriptions
// simple.)
func (gb *GrowthBook) Run(exp *Experiment) *Result {
	// Actually run the experiment.
	result := gb.doRun(exp, "")

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
func (gb *GrowthBook) doRun(exp *Experiment, featureID string) *Result {
	// 1. If exp.Variations has fewer than 2 variations, return default
	//    result.
	if len(exp.Variations) < 2 {
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 2. If context.Enabled is false, return default result.
	if !gb.context.Enabled {
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 3. If context.URL exists, check for query string override and use
	//    it if it exists.
	if gb.context.URL != nil {
		qsOverride := getQueryStringOverride(exp.Key, gb.context.URL, len(exp.Variations))
		if qsOverride != nil {
			return gb.getResult(exp, *qsOverride, false, featureID, nil)
		}
	}

	// 4. Return forced result if forced via context.
	force, forced := gb.context.ForcedVariations[exp.Key]
	if forced {
		return gb.getResult(exp, force, false, featureID, nil)
	}

	// 5. If exp.Active is set to false, return default result.
	if !exp.Active {
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 6. Get the user hash value and return if empty.
	hashAttribute := "id"
	if exp.HashAttribute != "" {
		hashAttribute = exp.HashAttribute
	}
	hashString := ""
	hashValue, ok := gb.context.Attributes[hashAttribute]
	if ok {
		hashString, _ = convertHashValue(hashValue)
	}
	if hashString == "" {
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 7. If exp.Namespace is set, return if not in range.
	if exp.Namespace != nil {
		if !inNamespace(hashString, exp.Namespace) {
			return gb.getResult(exp, -1, false, featureID, nil)
		}
	}

	// 8. If exp.Condition is set, return if it evaluates to false.
	if exp.Condition != nil {
		if !exp.Condition.Eval(gb.context.Attributes) {
			return gb.getResult(exp, -1, false, featureID, nil)
		}
	}

	// 9. Calculate bucket ranges for the variations and choose one.
	coverage := float64(1)
	if exp.Coverage != nil {
		coverage = *exp.Coverage
	}
	ranges := getBucketRanges(len(exp.Variations), coverage, exp.Weights)
	n := hash("", hashString+exp.Key, 1)
	assigned := chooseVariation(*n, ranges)

	// 10. If assigned == -1, return default result.
	if assigned == -1 {
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 11. If experiment has a forced variation, return it.
	if exp.Force != nil {
		return gb.getResult(exp, *exp.Force, false, featureID, nil)
	}

	// 12. If in QA mode, return default result.
	if gb.context.QAMode {
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 13. Build the result object.
	result := gb.getResult(exp, assigned, true, featureID, n)

	// 14. Fire tracking callback if required.
	gb.track(exp, result)

	return result
}

// Fire Context.TrackingCallback if it's set and the combination of
// hashAttribute, hashValue, experiment key, and variation ID has not
// been tracked before.
func (gb *GrowthBook) track(exp *Experiment, result *Result) {
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
func (gb *GrowthBook) GetAllResults() map[string]*Result {
	return gb.latestResults
}

// ClearSavedResults clears out any experiment results saved within a
// GrowthBook instance (used for deciding whether to send data to
// subscriptions).
func (gb *GrowthBook) ClearSavedResults() {
	gb.latestResults = map[string]*Result{}
}

// ClearTrackingData clears out records of calls to the experiment
// tracking callback.
func (gb *GrowthBook) ClearTrackingData() {
	gb.trackedExperiments = map[string]bool{}
}
