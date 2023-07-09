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

// GrowthBook is the main export of the SDK. A GrowthBook instance is
// created from a Context and is used for evaluating features and
// running in-line experiments.
type GrowthBook struct {
	data     *sharedData
	features *featureData
	attrs    Attributes
}

// Data shared between GrowthBook instances derived from the same
// context.
type sharedData struct {
	enabled             bool
	url                 *url.URL
	forcedVariations    ForcedVariationsMap
	qaMode              bool
	devMode             bool
	trackingCallback    ExperimentCallback
	onFeatureUsage      FeatureUsageCallback
	groups              map[string]bool
	apiHost             string
	clientKey           string
	overrides           ExperimentOverrides
	cacheTTL            time.Duration
	forcedFeatureValues map[string]interface{}
	attributeOverrides  Attributes
	trackedFeatures     map[string]interface{}
	trackedExperiments  map[string]bool
	nextSubscriptionID  subscriptionID
	subscriptions       map[subscriptionID]ExperimentCallback
	assigned            map[string]*Assignment
}

// Feature data, which needs to be held separately to allow sharing
// with the auto-update code.
type featureData struct {
	sync.RWMutex
	features      FeatureMap
	decryptionKey string
	ready         bool
}

// New creates a new GrowthBook instance.
func New(context *Context) *GrowthBook {
	// There is a little complexity here. The feature auto-refresh code
	// needs to keep track of some information about GrowthBook
	// instances to update them with new feature information, but we
	// want GrowthBook instances to be garbage collected normally. Go
	// doesn't have weak references (if it did, the auto-refresh code
	// could keep weak references to GrowthBook instances), so we have
	// to do something sneaky.
	//
	// The main GrowthBook instance wraps a featureData instance
	// which contains the feature information. The auto-refresh code
	// stores references only to the inner featureData data
	// structures. The main outer GrowthBook data structure has a
	// finalizer that handles removing the inner featureData instance
	// from the auto-refresh code.
	//
	// This means that the lifecycle of the relevant objects is:
	//
	//  1. featureData value created (here in New).
	//  2. GrowthBook value created, wrapping featureData value (here
	//     in New).
	//  3. GrowthBook instance is used...
	//  4. GrowthBook instance is subscribed to auto-refresh updates.
	//     This adds a reference to the inner featureData value to
	//     the auto-refresh code's data structures.
	//
	//  ... more use of GrowthBook instance ...
	//
	//  5. GrowthBook instance is unreferenced, so eligible for GC.
	//  6. Garbage collection.
	//  7. GrowthBook instance is collected by GC and its finalizer is
	//     run, which calls repoUnsubscribe. This removes the inner
	//     featureData instance from the auto-refresh code's data
	//     structures. (The finalizer resurrects the GrowthBook
	//     instance, so another cycle of GC is needed to collect it for
	//     real.)
	//  8. Both the main GrowthBook instance and the inner
	//     featureData instance are now unreferenced, so eligible for
	//     GC.
	//  9. Garbage collection.
	// 10. Main GrowthBook instance and inner featureData instance
	//     are collected.
	//
	// The end result of all this is that the auto-refresh code can keep
	// hold of the data that it needs to update instances with new
	// features, but those resources are freed correctly when users drop
	// references to instances of the public GrowthBook structure.

	if context == nil {
		// Default context.
		context = NewContext()
	}

	// Feature tracking information.
	features := &featureData{
		features:      context.features,
		decryptionKey: context.decryptionKey,
		ready:         len(context.features) != 0,
	}

	// data shared between GrowthBook instances.
	shared := &sharedData{
		enabled:             context.enabled,
		url:                 context.url,
		forcedVariations:    context.forcedVariations.Copy(),
		qaMode:              context.qaMode,
		devMode:             context.devMode,
		trackingCallback:    context.trackingCallback,
		onFeatureUsage:      context.onFeatureUsage,
		groups:              context.groups,
		apiHost:             context.apiHost,
		clientKey:           context.clientKey,
		overrides:           context.overrides.Copy(),
		cacheTTL:            context.cacheTTL,
		forcedFeatureValues: make(map[string]interface{}),
		attributeOverrides:  make(Attributes),
		trackedFeatures:     make(map[string]interface{}),
		trackedExperiments:  make(map[string]bool),
		subscriptions:       make(map[subscriptionID]ExperimentCallback),
		assigned:            make(map[string]*Assignment),
	}

	gb := &GrowthBook{
		data:     shared,
		features: features,
		attrs:    context.attributes,
	}

	runtime.SetFinalizer(gb, func(gb *GrowthBook) { repoUnsubscribe(gb) })
	if context.clientKey != "" {
		go gb.refresh(nil, true, false)
	}
	return gb
}

// Ready returns the ready flag, which indicates that features have
// been loaded.
func (gb *GrowthBook) Ready() bool {
	return gb.features.ready
}

// ForcedFeatures returns the current forced feature values.
func (gb *GrowthBook) ForcedFeatures() map[string]interface{} {
	return gb.data.forcedFeatureValues
}

// WithForcedFeatures updates the current forced feature values.
func (gb *GrowthBook) WithForcedFeatures(values map[string]interface{}) *GrowthBook {
	if values == nil {
		values = map[string]interface{}{}
	}
	gb.data.forcedFeatureValues = values
	return gb
}

// AttributeOverrides returns the current attribute overrides.
func (gb *GrowthBook) AttributeOverrides() Attributes {
	return gb.data.attributeOverrides
}

// WithAttributeOverrides updates the current attribute overrides.
func (gb *GrowthBook) WithAttributeOverrides(overrides Attributes) *GrowthBook {
	if overrides == nil {
		overrides = Attributes{}
	}
	gb.data.attributeOverrides = overrides
	return gb
}

// Enabled returns the current enabled status.
func (gb *GrowthBook) Enabled() bool { return gb.data.enabled }

// WithEnabled sets the enabled status.
func (gb *GrowthBook) WithEnabled(enabled bool) *GrowthBook {
	gb.data.enabled = enabled
	return gb
}

// Attributes returns the current attributes, possibly modified by
// overrides.
func (gb *GrowthBook) Attributes() Attributes {
	attrs := Attributes{}
	for id, v := range gb.attrs {
		attrs[id] = v
	}
	if gb.data.attributeOverrides != nil {
		for id, v := range gb.data.attributeOverrides {
			attrs[id] = v
		}
	}
	return attrs
}

// WithAttributes updates the current attributes. It returns a new
// GrowthBook instance sharing all non-attribute data with the
// original instance, but with different attributes.
func (gb *GrowthBook) WithAttributes(attrs Attributes) *GrowthBook {
	if attrs == nil {
		attrs = Attributes{}
	}
	return &GrowthBook{
		data:     gb.data,
		features: gb.features,
		attrs:    attrs,
	}
}

// Overrides returns the current experiment overrides.
func (gb *GrowthBook) Overrides() ExperimentOverrides {
	return gb.data.overrides
}

// WithOverrides sets the current experiment overrides.
func (gb *GrowthBook) WithOverrides(overrides ExperimentOverrides) *GrowthBook {
	if overrides == nil {
		overrides = ExperimentOverrides{}
	}
	gb.data.overrides = overrides
	return gb
}

// CacheTTL returns the current TTL for the feature cache.
func (gb *GrowthBook) CacheTTL() time.Duration {
	return gb.data.cacheTTL
}

// WithCacheTTL sets the TTL for the feature cache.
func (gb *GrowthBook) WithCacheTTL(ttl time.Duration) *GrowthBook {
	gb.data.cacheTTL = ttl
	return gb
}

// URL returns the current matching URL.
func (gb *GrowthBook) URL() *url.URL {
	return gb.data.url
}

// WithURL sets the current matching URL.
func (gb *GrowthBook) WithURL(url *url.URL) *GrowthBook {
	gb.data.url = url
	return gb
}

// WithFeatures explicitly updates the current features.
func (gb *GrowthBook) WithFeatures(features FeatureMap) *GrowthBook {
	gb.features.withFeatures(features)
	return gb
}

func (feats *featureData) withFeatures(features FeatureMap) {
	feats.Lock()
	defer feats.Unlock()
	ready := true
	if features == nil {
		features = FeatureMap{}
		ready = false
	}
	feats.features = features
	feats.ready = ready
}

func (feats *featureData) DecryptionKey() string {
	feats.RLock()
	defer feats.RUnlock()
	return feats.decryptionKey
}

func (feats *featureData) withDecryptionKey(key string) {
	feats.Lock()
	defer feats.Unlock()
	feats.decryptionKey = key
}

// Features returns the features in a GrowthBook's context.
func (gb *GrowthBook) Features() FeatureMap {
	return gb.features.getFeatures()
}

func (feats *featureData) getFeatures() FeatureMap {
	feats.RLock()
	defer feats.RUnlock()
	return feats.features
}

// WithEncryptedFeatures updates the features in a GrowthBook's
// context from encrypted data.
func (gb *GrowthBook) WithEncryptedFeatures(encrypted string, key string) (*GrowthBook, error) {
	err := gb.features.withEncryptedFeatures(encrypted, key)
	return gb, err
}

func (feats *featureData) withEncryptedFeatures(encrypted string, key string) error {
	feats.Lock()
	defer feats.Unlock()

	if key == "" {
		key = feats.decryptionKey
	}

	featuresJson, err := decrypt(encrypted, key)
	var features FeatureMap
	if err == nil {
		features = ParseFeatureMap([]byte(featuresJson))
		if features != nil {
			feats.features = features
			feats.ready = true
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
	if forcedVariations == nil {
		forcedVariations = ForcedVariationsMap{}
	}
	gb.data.forcedVariations = forcedVariations
	return gb
}

func (gb *GrowthBook) ForceVariation(key string, variation int) {
	gb.ForceVariation(key, variation)
}

func (gb *GrowthBook) UnforceVariation(key string) {
	gb.UnforceVariation(key)
}

// WithQAMode can be used to enable or disable the QA mode for a
// context.
func (gb *GrowthBook) WithQAMode(qaMode bool) *GrowthBook {
	gb.data.qaMode = qaMode
	return gb
}

// WithDevMode can be used to enable or disable the development mode
// for a context.
func (gb *GrowthBook) WithDevMode(devMode bool) *GrowthBook {
	gb.data.devMode = devMode
	return gb
}

// WithTrackingCallback is used to set a tracking callback for a
// context.
func (gb *GrowthBook) WithTrackingCallback(callback ExperimentCallback) *GrowthBook {
	gb.data.trackingCallback = callback
	return gb
}

// WithFeatureUsageCallback is used to set a feature usage callback
// for a context.
func (gb *GrowthBook) WithFeatureUsageCallback(callback FeatureUsageCallback) *GrowthBook {
	gb.data.onFeatureUsage = callback
	return gb
}

// WithGroups sets the groups map of a context.
func (gb *GrowthBook) WithGroups(groups map[string]bool) *GrowthBook {
	if groups == nil {
		groups = map[string]bool{}
	}
	gb.data.groups = groups
	return gb
}

// WithAPIHost sets the API host of a context.
func (gb *GrowthBook) WithAPIHost(host string) *GrowthBook {
	gb.data.apiHost = host
	return gb
}

// WithClientKey sets the API client key of a context.
func (gb *GrowthBook) WithClientKey(key string) *GrowthBook {
	gb.data.clientKey = key
	return gb
}

// WithDecryptionKey sets the decryption key of a context.
func (gb *GrowthBook) WithDecryptionKey(key string) *GrowthBook {
	gb.features.withDecryptionKey(key)
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
	return gb.EvalFeature(key).On
}

// IsOff determines whether a feature is off.
func (gb *GrowthBook) IsOff(key string) bool {
	return gb.EvalFeature(key).Off
}

// GetFeatureValue returns the result for a feature identified by a
// string feature key, with an explicit default.
func (gb *GrowthBook) GetFeatureValue(key string, defaultValue interface{}) interface{} {
	featureValue := gb.EvalFeature(key).Value
	if featureValue != nil {
		return featureValue
	}
	return defaultValue
}

// Deprecated: Use EvalFeature instead. Feature returns the result for
// a feature identified by a string feature key.
func (gb *GrowthBook) Feature(key string) *FeatureResult {
	return gb.EvalFeature(key)
}

// EvalFeature returns the result for a feature identified by a string
// feature key.
func (gb *GrowthBook) EvalFeature(id string) *FeatureResult {
	gb.features.RLock()
	defer gb.features.RUnlock()

	// Global override.
	if len(gb.data.forcedFeatureValues) != 0 {
		if override, ok := gb.data.forcedFeatureValues[id]; ok {
			logInfo("Global override", id, override)
			return gb.getFeatureResult(id, override, OverrideResultSource, "", nil, nil)
		}
	}

	// Handle unknown features.
	feature, ok := gb.features.features[id]
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
	gb.features.RLock()
	defer gb.features.RUnlock()

	result := gb.doRun(exp, "")
	gb.fireSubscriptions(exp, result)
	return result
}

// Subscribe adds a callback that is called every time GrowthBook.Run
// is called. This is different from the tracking callback since it
// also fires when a user is not included in an experiment.
func (gb *GrowthBook) Subscribe(callback ExperimentCallback) func() {
	gb.features.Lock()
	defer gb.features.Unlock()

	id := gb.data.nextSubscriptionID
	gb.data.subscriptions[id] = callback
	gb.data.nextSubscriptionID++
	return func() {
		delete(gb.data.subscriptions, id)
	}
}

// GetAllResults returns a map containing all the latest results from
// all experiments that have been run, indexed by the experiment key.
func (gb *GrowthBook) GetAllResults() map[string]*Assignment {
	return gb.data.assigned
}

// ClearSavedResults clears out any experiment results saved within a
// GrowthBook instance (used for deciding whether to send data to
// subscriptions).
func (gb *GrowthBook) ClearSavedResults() {
	gb.data.assigned = make(map[string]*Assignment)
}

// ClearTrackingData clears out records of calls to the experiment
// tracking callback.
func (gb *GrowthBook) ClearTrackingData() {
	gb.data.trackedExperiments = make(map[string]bool)
}

// GetAPIInfo gets the hostname and client key for GrowthBook API
// access.
func (gb *GrowthBook) GetAPIInfo() (string, string) {
	apiHost := gb.data.apiHost
	if apiHost == "" {
		apiHost = "https://cdn.growthbook.io"
	}

	return strings.TrimRight(apiHost, "/"), gb.data.clientKey
}

type FeatureRepoOptions struct {
	AutoRefresh bool
	Timeout     time.Duration
	SkipCache   bool
}

func (gb *GrowthBook) LoadFeatures(options *FeatureRepoOptions) {
	gb.refresh(options, true, true)
	if options != nil && options.AutoRefresh {
		repoSubscribe(gb)
	}
}

func (gb *GrowthBook) LatestFeatureUpdate() *time.Time {
	return repoLatestUpdate(gb)
}

func (gb *GrowthBook) RefreshFeatures(options *FeatureRepoOptions) {
	gb.refresh(options, false, true)
}

//-- PRIVATE FUNCTIONS START HERE ----------------------------------------------

func (gb *GrowthBook) refresh(
	options *FeatureRepoOptions, allowStale bool, updateInstance bool) {

	if gb.data.clientKey == "" {
		logError("Missing clientKey")
		return
	}
	var timeout time.Duration
	skipCache := gb.data.devMode
	if options != nil {
		timeout = options.Timeout
		skipCache = skipCache || options.SkipCache
	}
	configureCacheStaleTTL(gb.data.cacheTTL)
	repoRefreshFeatures(gb, timeout, skipCache, allowStale, updateInstance)
}

func (gb *GrowthBook) trackFeatureUsage(key string, res *FeatureResult) {
	// Don't track feature usage that was forced via an override.
	if res.Source == OverrideResultSource {
		return
	}

	// Only track a feature once, unless the assigned value changed.
	if saved, ok := gb.data.trackedFeatures[key]; ok && reflect.DeepEqual(saved, res.Value) {
		return
	}
	gb.data.trackedFeatures[key] = res.Value

	// Fire user-supplied callback
	if gb.data.onFeatureUsage != nil {
		gb.data.onFeatureUsage(key, res)
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
	storedResult, exists := gb.data.assigned[exp.Key]
	if exists {
		if storedResult.Result.InExperiment != result.InExperiment ||
			storedResult.Result.VariationID != result.VariationID {
			changed = true
		}
	}

	// Store the experiment result.
	gb.data.assigned[exp.Key] = &Assignment{exp, result}

	// If the result changed, trigger all subscriptions.
	if changed || !exists {
		for _, sub := range gb.data.subscriptions {
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
	if !gb.data.enabled {
		logInfo("Context disabled", exp.Key)
		return gb.getResult(exp, -1, false, featureID, nil)
	}

	// 2.5. Merge in experiment overrides from the context
	exp = gb.mergeOverrides(exp)

	// 3. If a variation is forced from a querystring, return the forced
	//    variation.
	if gb.data.url != nil {
		qsOverride := getQueryStringOverride(exp.Key, gb.data.url, len(exp.Variations))
		if qsOverride != nil {
			logInfo("Force via querystring", exp.Key, qsOverride)
			return gb.getResult(exp, *qsOverride, false, featureID, nil)
		}
	}

	// 4. If a variation is forced in the context, return the forced
	//    variation.
	if gb.data.forcedVariations != nil {
		force, forced := gb.data.forcedVariations[exp.Key]
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
		if !exp.Condition.Eval(gb.attrs) {
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
	if exp.URLPatterns != nil && !isURLTargeted(gb.data.url, exp.URLPatterns) {
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
	if gb.data.qaMode {
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
	if gb.data.overrides == nil {
		return exp
	}
	if override, ok := gb.data.overrides[exp.Key]; ok {
		exp = exp.applyOverride(override)
	}
	return exp
}

// Fire Context.TrackingCallback if it's set and the combination of
// hashAttribute, hashValue, experiment key, and variation ID has not
// been tracked before.
func (gb *GrowthBook) track(exp *Experiment, result *Result) {
	if gb.data.trackingCallback == nil {
		return
	}

	// Make sure tracking callback is only fired once per unique
	// experiment.
	key := result.HashAttribute + result.HashValue +
		exp.Key + strconv.Itoa(result.VariationID)
	if _, exists := gb.data.trackedExperiments[key]; exists {
		return
	}

	gb.data.trackedExperiments[key] = true
	gb.data.trackingCallback(exp, result)
}

func (gb *GrowthBook) getHashAttribute(attr string) (string, string) {
	hashAttribute := "id"
	if attr != "" {
		hashAttribute = attr
	}

	var hashValue interface{}
	ok := false
	if gb.data.attributeOverrides != nil {
		hashValue, ok = gb.data.attributeOverrides[hashAttribute]
	}
	if !ok {
		if gb.attrs != nil {
			hashValue, ok = gb.attrs[hashAttribute]
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
		if val, ok := gb.data.groups[g]; ok && val {
			return true
		}
	}
	return false
}

func (gb *GrowthBook) urlIsValid(urlRegexp *regexp.Regexp) bool {
	regurl := gb.data.url
	if regurl == nil {
		return false
	}

	return urlRegexp.MatchString(regurl.String()) ||
		urlRegexp.MatchString(regurl.Path)
}
