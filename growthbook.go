// Package growthbook provides a Go SDK for the GrowthBook A/B testing
// and feature flagging service.
package growthbook

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type subscriptionID uint

// Assignment is used for recording subscription information.
type Assignment struct {
	Experiment *Experiment
	Result     *Result
}

// GrowthBook is the main export of the SDK.
type GrowthBook struct {
	inner *growthBookData
}

type growthBookData struct {
	sync.RWMutex
	context             *Context
	forcedFeatureValues map[string]interface{}
	attributeOverrides  Attributes
	trackedFeatures     map[string]interface{}
	trackedExperiments  map[string]bool
	nextSubscriptionID  subscriptionID
	subscriptions       map[subscriptionID]ExperimentCallback
	assigned            map[string]*Assignment
	ready               bool
}

// New creates a new GrowthBook instance.
func New(context *Context) *GrowthBook {
	// There is a little complexity here. The feature auto-refresh code
	// needs to keep track of information about GrowthBook instances to
	// update them with new feature information, but we want GrowthBook
	// instances to be garbage collected normally. Go doesn't have weak
	// references (if it did, the auto-refresh code could keep weak
	// references to GrowthBook instances), so we have to do something
	// sneaky.
	//
	// The main GrowthBook instance is a wrapper around a growthBookData
	// instance. The auto-refresh code stores references only to the
	// inner growthBookData data structures. The main outer GrowthBook
	// data structure has a finalizer that handles removing the inner
	// growthBookData instance from the auto-refresh code.
	//
	// This means that the lifecycle of the relevant objects is:
	//
	//  1. growthBookData value created (here in New).
	//  2. GrowthBook value created, wrapping growthBookData value (here
	//     in New).
	//  3. GrowthBook instance is used...
	//  4. GrowthBook instance is subscribed to auto-refresh updates.
	//     This adds a reference to the inner growthBookData value to
	//     the auto-refresh code's data structures.
	//
	//  ... more use of GrowthBook instance ...
	//
	//  5. GrowthBook instance is unreferenced, so eligible for GC.
	//  6. Garbage collection.
	//  7. GrowthBook instance is collected by GC and its finalizer is
	//     run, which calls RepoUnsubscribe. This removes the inner
	//     growthBookData instance from the auto-refresh code's data
	//     structures. (The finalizer resurrects the GrowthBook
	//     instance, so another cycle of GC is needed to collect it for
	//     real.)
	//  8. Both the main GrowthBook instance and the inner
	//     growthBookData instance are now unreferenced, so eligible for
	//     GC.
	//  9. Garbage collection.
	// 10. Main GrowthBook instance and inner growthBookData instance
	//     are collected.
	//
	// The end result of all this is that the auto-refresh code can keep
	// hold of the data that it needs to update instances with new
	// features, but those resources are freed correctly when users drop
	// references to instances of the public GrowthBook structure.

	if context == nil {
		context = NewContext()
	}
	inner := &growthBookData{
		context:             context,
		forcedFeatureValues: nil,
		attributeOverrides:  nil,
		trackedFeatures:     make(map[string]interface{}),
		trackedExperiments:  make(map[string]bool),
		nextSubscriptionID:  1,
		subscriptions:       make(map[subscriptionID]ExperimentCallback),
		assigned:            make(map[string]*Assignment),
	}
	gb := &GrowthBook{inner}
	runtime.SetFinalizer(gb, func(gb *GrowthBook) { RepoUnsubscribe(gb) })
	if context.ClientKey != "" {
		go gb.refresh(nil, true, false)
	}
	return gb
}

// Ready returns the ready flag, which indicates that features have
// been loaded.
func (gb *GrowthBook) Ready() bool {
	gb.inner.RLock()
	defer gb.inner.RUnlock()
	return gb.inner.ready
}

// WithForcedFeatures updates the current forced feature values.
func (gb *GrowthBook) WithForcedFeatures(values map[string]interface{}) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.forcedFeatureValues = values
	return gb
}

// WithAttributeOverrides updates the current attribute overrides.
func (gb *GrowthBook) WithAttributeOverrides(overrides Attributes) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.attributeOverrides = overrides
	return gb
}

// WithEnabled sets the enabled flag in a GrowthBook's context.
func (gb *GrowthBook) WithEnabled(enabled bool) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.Enabled = enabled
	return gb
}

// WithAttributes updates the attributes in a GrowthBook's context.
func (gb *GrowthBook) WithAttributes(attrs Attributes) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.Attributes = attrs
	return gb
}

// Attributes returns the attributes in a GrowthBook's context,
// possibly modified by overrides.
func (gb *GrowthBook) Attributes() Attributes {
	gb.inner.RLock()
	defer gb.inner.RUnlock()

	attrs := Attributes{}
	for id, v := range gb.inner.context.Attributes {
		attrs[id] = v
	}
	if gb.inner.attributeOverrides != nil {
		for id, v := range gb.inner.attributeOverrides {
			attrs[id] = v
		}
	}
	return attrs
}

// WithURL sets the URL in a GrowthBook's context.
func (gb *GrowthBook) WithURL(url *url.URL) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.URL = url
	return gb
}

// WithFeatures updates the features in a GrowthBook's context.
func (gb *GrowthBook) WithFeatures(features FeatureMap) *GrowthBook {
	gb.inner.withFeatures(features)
	return gb
}

func (inner *growthBookData) withFeatures(features FeatureMap) {
	inner.Lock()
	defer inner.Unlock()

	inner.context.Features = features
	inner.ready = true
}

// Features returns the features in a GrowthBook's context.
func (gb *GrowthBook) Features() FeatureMap {
	return gb.inner.features()
}

func (inner *growthBookData) features() FeatureMap {
	inner.RLock()
	defer inner.RUnlock()
	return inner.context.Features
}

// WithEncryptedFeatures updates the features in a GrowthBook's
// context from encrypted data.
func (gb *GrowthBook) WithEncryptedFeatures(encrypted string, key string) (*GrowthBook, error) {
	err := gb.inner.withEncryptedFeatures(encrypted, key)
	return gb, err
}

func (inner *growthBookData) withEncryptedFeatures(encrypted string, key string) error {
	inner.Lock()
	defer inner.Unlock()

	if key == "" {
		key = inner.context.DecryptionKey
	}
	featuresJson, err := decrypt(encrypted, key)

	var features FeatureMap
	if err == nil {
		features = ParseFeatureMap([]byte(featuresJson))
		if features != nil {
			inner.context.Features = ParseFeatureMap([]byte(featuresJson))
		}
	}
	if err != nil || features == nil {
		err = errors.New("failed to decode encrypted features")
	}
	return err
}

// WithForcedVariations sets the forced variations in a GrowthBook's
// context.
func (gb *GrowthBook) WithForcedVariations(forcedVariations ForcedVariationsMap) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.ForcedVariations = forcedVariations
	return gb
}

func (gb *GrowthBook) ForceVariation(key string, variation int) {
	gb.inner.RLock()
	defer gb.inner.RUnlock()
	gb.inner.context.ForceVariation(key, variation)
}

func (gb *GrowthBook) UnforceVariation(key string) {
	gb.inner.RLock()
	defer gb.inner.RUnlock()
	gb.inner.context.UnforceVariation(key)
}

// WithQAMode can be used to enable or disable the QA mode for a
// context.
func (gb *GrowthBook) WithQAMode(qaMode bool) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.QAMode = qaMode
	return gb
}

// WithDevMode can be used to enable or disable the development mode
// for a context.
func (gb *GrowthBook) WithDevMode(devMode bool) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.DevMode = devMode
	return gb
}

// WithTrackingCallback is used to set a tracking callback for a
// context.
func (gb *GrowthBook) WithTrackingCallback(callback ExperimentCallback) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.TrackingCallback = callback
	return gb
}

// WithFeatureUsageCallback is used to set a feature usage callback
// for a context.
func (gb *GrowthBook) WithFeatureUsageCallback(callback FeatureUsageCallback) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.OnFeatureUsage = callback
	return gb
}

// WithGroups sets the groups map of a context.
func (gb *GrowthBook) WithGroups(groups map[string]bool) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.Groups = groups
	return gb
}

// WithAPIHost sets the API host of a context.
func (gb *GrowthBook) WithAPIHost(host string) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.APIHost = host
	return gb
}

// WithClientKey sets the API client key of a context.
func (gb *GrowthBook) WithClientKey(key string) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.ClientKey = key
	return gb
}

// WithDecryptionKey sets the decryption key of a context.
func (gb *GrowthBook) WithDecryptionKey(key string) *GrowthBook {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	gb.inner.context.DecryptionKey = key
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

// IsOn determines whether a feature is on.
func (gb *GrowthBook) IsOn(key string) bool {
	gb.inner.RLock()
	defer gb.inner.RUnlock()
	return gb.EvalFeature(key).On
}

// IsOff determines whether a feature is off.
func (gb *GrowthBook) IsOff(key string) bool {
	gb.inner.RLock()
	defer gb.inner.RUnlock()
	return gb.EvalFeature(key).Off
}

// GetFeatureValue returns the result for a feature identified by a
// string feature key, with an explicit default.
func (gb *GrowthBook) GetFeatureValue(key string, defaultValue interface{}) interface{} {
	gb.inner.RLock()
	defer gb.inner.RUnlock()
	featureValue := gb.EvalFeature(key).Value
	if featureValue != nil {
		return featureValue
	}
	return defaultValue
}

// Feature returns the result for a feature identified by a string
// feature key. (DEPRECATED: Use EvalFeature instead.)
func (gb *GrowthBook) Feature(key string) *FeatureResult {
	gb.inner.RLock()
	defer gb.inner.RUnlock()
	return gb.EvalFeature(key)
}

// EvalFeature returns the result for a feature identified by a string
// feature key.
func (gb *GrowthBook) EvalFeature(id string) *FeatureResult {
	gb.inner.RLock()
	defer gb.inner.RUnlock()

	// Global override.
	if gb.inner.forcedFeatureValues != nil {
		if override, ok := gb.inner.forcedFeatureValues[id]; ok {
			logInfo("Global override", id, override)
			return gb.getFeatureResult(id, override, OverrideResultSource, "", nil, nil)
		}
	}

	// Handle unknown features.
	feature, ok := gb.inner.context.Features[id]
	if !ok {
		logWarn("Unknown feature", id)
		return gb.getFeatureResult(id, nil, UnknownResultSource, "", nil, nil)
	}

	// Loop through the feature rules.
	for _, rule := range feature.Rules {
		// If the rule has a condition and the condition does not pass,
		// skip this rule.
		if rule.Condition != nil && !rule.Condition.Eval(gb.Attributes()) {
			logInfo("Skip rule because of condition", id, rule)
			continue
		}

		// Apply any filters for who is included (e.g. namespaces).
		if rule.Filters != nil && gb.isFilteredOut(rule.Filters) {
			logInfo("Skip rule because of filters", id, rule)
			continue
		}

		// Feature value is being forced.
		if rule.Force != nil {
			// If this is a percentage rollout, skip if not included.
			seed := id
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
				logInfo("Skip rule because user not included in rollout", id, rule)
				continue
			}

			// Return forced feature result.
			logInfo("Force value from rule", id, rule)
			return gb.getFeatureResult(id, rule.Force, ForceResultSource, rule.ID, nil, nil)
		}

		if rule.Variations == nil {
			logWarn("Skip invalid rule", id, rule)
			continue
		}

		// Otherwise, convert the rule to an Experiment object, copying
		// values from the rule as necessary.
		exp := experimentFromFeatureRule(id, rule)

		// Run the experiment.
		result := gb.doRun(exp, id)
		gb.fireSubscriptions(exp, result)

		// Only return a value if the user is part of the experiment.
		// gb.fireSubscriptions(experiment, result)
		if result.InExperiment && !result.Passthrough {
			return gb.getFeatureResult(id, result.Value, ExperimentResultSource, rule.ID, exp, result)
		}
	}

	// Fall back to using the default value.
	logInfo("Use default value", id, feature.DefaultValue)
	return gb.getFeatureResult(id, feature.DefaultValue, DefaultValueResultSource, "", nil, nil)
}

// Run an experiment. (Uses doRun to make wrapping for subscriptions
// simple.)
func (gb *GrowthBook) Run(exp *Experiment) *Result {
	gb.inner.RLock()
	defer gb.inner.RUnlock()

	result := gb.doRun(exp, "")
	gb.fireSubscriptions(exp, result)
	return result
}

// Subscribe adds a callback that is called every time GrowthBook.Run
// is called. This is different from the tracking callback since it
// also fires when a user is not included in an experiment.
func (gb *GrowthBook) Subscribe(callback ExperimentCallback) func() {
	gb.inner.Lock()
	defer gb.inner.Unlock()

	id := gb.inner.nextSubscriptionID
	gb.inner.subscriptions[id] = callback
	gb.inner.nextSubscriptionID++
	return func() {
		delete(gb.inner.subscriptions, id)
	}
}

// GetAllResults returns a map containing all the latest results from
// all experiments that have been run, indexed by the experiment key.
func (gb *GrowthBook) GetAllResults() map[string]*Assignment {
	gb.inner.RLock()
	defer gb.inner.RUnlock()
	return gb.inner.assigned
}

// ClearSavedResults clears out any experiment results saved within a
// GrowthBook instance (used for deciding whether to send data to
// subscriptions).
func (gb *GrowthBook) ClearSavedResults() {
	gb.inner.Lock()
	defer gb.inner.Unlock()
	gb.inner.assigned = make(map[string]*Assignment)
}

// ClearTrackingData clears out records of calls to the experiment
// tracking callback.
func (gb *GrowthBook) ClearTrackingData() {
	gb.inner.Lock()
	defer gb.inner.Unlock()
	gb.inner.trackedExperiments = make(map[string]bool)
}

// GetAPIInfo gets the hostname and client key for GrowthBook API
// access.
func (gb *GrowthBook) GetAPIInfo() (string, string) {
	gb.inner.RLock()
	defer gb.inner.RUnlock()

	apiHost := gb.inner.context.APIHost
	if apiHost == "" {
		apiHost = "https://cdn.growthbook.io"
	}

	return strings.TrimRight(apiHost, "/"), gb.inner.context.ClientKey
}

type FeatureRepoOptions struct {
	AutoRefresh bool
	Timeout     time.Duration
	SkipCache   bool
}

func (gb *GrowthBook) LoadFeatures(options *FeatureRepoOptions) {
	gb.refresh(options, true, true)
	if options != nil && options.AutoRefresh {
		RepoSubscribe(gb)
	}
}

func (gb *GrowthBook) RefreshFeatures(options *FeatureRepoOptions) {
	gb.refresh(options, false, true)
}

func (gb *GrowthBook) refresh(
	options *FeatureRepoOptions, allowStale bool, updateInstance bool) {

	if gb.inner.context.ClientKey == "" {
		logError("Missing clientKey")
		return
	}
	var timeout time.Duration
	skipCache := gb.inner.context.DevMode
	if options != nil {
		timeout = options.Timeout
		skipCache = skipCache || options.SkipCache
	}
	RepoRefreshFeatures(gb, timeout, skipCache, allowStale, updateInstance)
}

//-- PRIVATE FUNCTIONS START HERE ----------------------------------------------

func (gb *GrowthBook) trackFeatureUsage(key string, res *FeatureResult) {
	// Don't track feature usage that was forced via an override.
	if res.Source == OverrideResultSource {
		return
	}

	// Only track a feature once, unless the assigned value changed.
	if saved, ok := gb.inner.trackedFeatures[key]; ok && reflect.DeepEqual(saved, res.Value) {
		return
	}
	gb.inner.trackedFeatures[key] = res.Value

	// Fire user-supplied callback
	if gb.inner.context.OnFeatureUsage != nil {
		gb.inner.context.OnFeatureUsage(key, res)
	}
}

func (gb *GrowthBook) getFeatureResult(
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

	gb.trackFeatureUsage(key, &retval)

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
	hashAttribute, hashString := gb.getHashAttribute(exp.HashAttribute)

	var meta *VariationMeta
	if exp.Meta != nil {
		if variationIndex < len(exp.Meta) {
			meta = &exp.Meta[variationIndex]
		}
	}

	// Return
	var value FeatureValue
	if variationIndex < len(exp.Variations) {
		value = exp.Variations[variationIndex]
	}
	key := fmt.Sprint(variationIndex)
	name := ""
	passthrough := false
	if meta != nil {
		if meta.Key != "" {
			key = meta.Key
		}
		if meta.Name != "" {
			name = meta.Name
		}
		passthrough = meta.Passthrough
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
		Name:          name,
		Passthrough:   passthrough,
	}
}

func (gb *GrowthBook) fireSubscriptions(exp *Experiment, result *Result) {
	// Determine whether the result changed from the last stored result
	// for the experiment.
	changed := false
	storedResult, exists := gb.inner.assigned[exp.Key]
	if exists {
		if storedResult.Result.InExperiment != result.InExperiment ||
			storedResult.Result.VariationID != result.VariationID {
			changed = true
		}
	}

	// Store the experiment result.
	gb.inner.assigned[exp.Key] = &Assignment{exp, result}

	// If the result changed, trigger all subscriptions.
	if changed || !exists {
		for _, sub := range gb.inner.subscriptions {
			sub(exp, result)
		}
	}
}

// Worker function to run an experiment.
func (gb *GrowthBook) doRun(exp *Experiment, featureID string) *Result {
	// 1. If experiment has fewer than two variations, return default
	//    result.
	if len(exp.Variations) < 2 {
		logWarn("Invalid experiment", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 2. If the context is disabled, return default result.
	if !gb.inner.context.Enabled {
		logInfo("Context disabled", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 2.5. Merge in experiment overrides from the context
	exp = gb.mergeOverrides(exp)

	// 3. If a variation is forced from a querystring, return the forced
	//    variation.
	if gb.inner.context.URL != nil {
		qsOverride := getQueryStringOverride(exp.Key, gb.inner.context.URL, len(exp.Variations))
		if qsOverride != nil {
			logInfo("Force via querystring", exp.Key, qsOverride)
			return gb.getResult(exp, *qsOverride, false, featureID, nil)
		}
	}

	// 4. If a variation is forced in the context, return the forced
	//    variation.
	if gb.inner.context.ForcedVariations != nil {
		force, forced := gb.inner.context.ForcedVariations[exp.Key]
		if forced {
			logInfo("Forced variation", exp.Key, force)
			return gb.getResult(exp, force, false, featureID, nil)
		}
	}

	// 5. Exclude inactive experiments and return default result.
	if exp.Status == DraftStatus || !exp.Active {
		logInfo("Skip because inactive", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 6. Get the user hash value and return if empty.
	_, hashString := gb.getHashAttribute(exp.HashAttribute)
	if hashString == "" {
		logInfo("Skip because of missing hash attribute", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 7. If exp.Namespace is set, return if not in range.
	if exp.Filters != nil {
		if gb.isFilteredOut(exp.Filters) {
			logInfo("Skip because of filters", exp.Key)
			return gb.getResult(exp, -1, false, featureID, nil)
		}
	} else if exp.Namespace != nil {
		if !exp.Namespace.inNamespace(hashString) {
			logInfo("Skip because of namespace", exp.Key)
			return gb.getResult(exp, -1, false, featureID, nil)
		}
	}

	// 7.5. Exclude if include function returns false.
	if exp.Include != nil && !exp.Include() {
		logInfo("Skip because of include function", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 8. Exclude if condition is false.
	if exp.Condition != nil {
		if !exp.Condition.Eval(gb.inner.context.Attributes) {
			logInfo("Skip because of condition", exp.Key)
			return gb.getResult(exp, -1, false, featureID, nil)
		}
	}

	// 8.1. Exclude if user is not in a required group.
	if exp.Groups != nil && !gb.hasGroupOverlap(exp.Groups) {
		logInfo("Skip because of groups", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 8.2. Old style URL targeting.
	if exp.URL != nil && !gb.urlIsValid(exp.URL) {
		logInfo("Skip because of URL", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 8.3. New, more powerful URL targeting
	if exp.URLPatterns != nil && !isURLTargeted(gb.inner.context.URL, exp.URLPatterns) {
		logInfo("Skip because of URL targeting", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 9. Calculate bucket ranges for the variations and choose one.
	seed := exp.Key
	if exp.Seed != "" {
		seed = exp.Seed
	}
	hv := 1
	if exp.HashVersion != 0 {
		hv = exp.HashVersion
	}
	n := hash(seed, hashString, hv)
	if n == nil {
		logWarn("Skip because of invalid hash version", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}
	coverage := float64(1)
	if exp.Coverage != nil {
		coverage = *exp.Coverage
	}
	ranges := exp.Ranges
	if ranges == nil {
		ranges = getBucketRanges(len(exp.Variations), coverage, exp.Weights)
	}
	assigned := chooseVariation(*n, ranges)

	// 10. If assigned == -1, return default result.
	if assigned == -1 {
		logInfo("Skip because of coverage", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 11. If experiment has a forced variation, return it.
	if exp.Force != nil {
		return gb.getResult(exp, *exp.Force, false, featureID, nil)
	}

	// 12. If in QA mode, return default result.
	if gb.inner.context.QAMode {
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 12.5. Exclude if experiment is stopped.
	if exp.Status == StoppedStatus {
		logInfo("Skip because stopped", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 13. Build the result object.
	result := gb.getResult(exp, assigned, true, featureID, n)

	// 14. Fire tracking callback if required.
	gb.track(exp, result)

	logInfo("In experiment", fmt.Sprintf("%s[%d]", exp.Key, result.VariationID))
	return result
}

func (gb *GrowthBook) mergeOverrides(exp *Experiment) *Experiment {
	if gb.inner.context.Overrides == nil {
		return exp
	}
	if override, ok := gb.inner.context.Overrides[exp.Key]; ok {
		exp = exp.applyOverride(override)
	}
	return exp
}

// Fire Context.TrackingCallback if it's set and the combination of
// hashAttribute, hashValue, experiment key, and variation ID has not
// been tracked before.
func (gb *GrowthBook) track(exp *Experiment, result *Result) {
	if gb.inner.context.TrackingCallback == nil {
		return
	}

	// Make sure tracking callback is only fired once per unique
	// experiment.
	key := result.HashAttribute + result.HashValue +
		exp.Key + strconv.Itoa(result.VariationID)
	if _, exists := gb.inner.trackedExperiments[key]; exists {
		return
	}

	gb.inner.trackedExperiments[key] = true
	gb.inner.context.TrackingCallback(exp, result)
}

func (gb *GrowthBook) getHashAttribute(attr string) (string, string) {
	hashAttribute := "id"
	if attr != "" {
		hashAttribute = attr
	}

	var hashValue interface{}
	ok := false
	if gb.inner.attributeOverrides != nil {
		hashValue, ok = gb.inner.attributeOverrides[hashAttribute]
	}
	if !ok {
		if gb.inner.context.Attributes != nil {
			hashValue, ok = gb.inner.context.Attributes[hashAttribute]
		} else if gb.inner.context.UserAttributes != nil {
			hashValue, ok = gb.inner.context.UserAttributes[hashAttribute]
		}
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

func (gb *GrowthBook) isFilteredOut(filters []Filter) bool {
	for _, filter := range filters {
		_, hashValue := gb.getHashAttribute(filter.Attribute)
		if hashValue == "" {
			return true
		}
		hv := 2
		if filter.HashVersion != 0 {
			hv = filter.HashVersion
		}
		n := hash(filter.Seed, hashValue, hv)
		if n == nil {
			return true
		}
		if filter.Ranges != nil {
			inRange := false
			for _, rng := range filter.Ranges {
				if rng.InRange(*n) {
					inRange = true
					break
				}
			}
			if !inRange {
				return true
			}
		}
	}
	return false
}

func (gb *GrowthBook) hasGroupOverlap(groups []string) bool {
	for _, g := range groups {
		if val, ok := gb.inner.context.Groups[g]; ok && val {
			return true
		}
	}
	return false
}

func (gb *GrowthBook) urlIsValid(urlRegexp *regexp.Regexp) bool {
	url := gb.inner.context.URL
	if url == nil {
		return false
	}

	return urlRegexp.MatchString(url.String()) ||
		urlRegexp.MatchString(url.Path)
}
