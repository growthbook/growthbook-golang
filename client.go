// Package growthbook provides a Go SDK for the GrowthBook A/B testing
// and feature flagging service.
package growthbook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/barkimedes/go-deepcopy"
)

type subscriptionID uint

// Assignment is used for recording subscription information.
type Assignment struct {
	Experiment *Experiment
	Result     *Result
}

func (a *Assignment) clone() *Assignment {
	return &Assignment{
		Experiment: a.Experiment.clone(),
		Result:     deepcopy.MustAnything(a.Result).(*Result),
	}
}

type assignments map[string]*Assignment

func (as assignments) clone() assignments {
	retval := assignments{}
	for k, v := range as {
		retval[k] = v.clone()
	}
	return retval
}

type subscriptions map[subscriptionID]ExperimentCallback

func (s subscriptions) clone() subscriptions {
	retval := subscriptions{}
	for id, cb := range s {
		retval[id] = cb
	}
	return retval
}

// GrowthBook is the main export of the SDK. A GrowthBook instance is
// created from a Context and is used for evaluating features and
// running in-line experiments.
type Client struct {
	opt                 *Options
	forcedVariations    ForcedVariationsMap
	overrides           ExperimentOverrides
	forcedFeatureValues map[string]interface{}
	attributeOverrides  Attributes
	trackedFeatures     map[string]interface{}
	trackedExperiments  map[string]bool
	nextSubscriptionID  subscriptionID
	subscriptions       subscriptions
	assigned            assignments
	features            *featureData
}

// Feature data, which needs to be held separately to allow sharing
// with the auto-update code.
type featureData struct {
	sync.RWMutex
	features      FeatureMap
	decryptionKey string
	ready         bool
}

func (feats *featureData) clone() *featureData {
	feats.RLock()
	defer feats.RUnlock()

	return &featureData{
		features:      feats.features.clone(),
		decryptionKey: feats.decryptionKey,
		ready:         feats.ready,
	}
}

// NewClient creates a new GrowthBook instance.
func NewClient(opt *Options) *Client {
	return NewClientContext(context.Background(), opt)
}

// NewClientContext creates a new GrowthBook instance with an explicit
// Go context.
func NewClientContext(ctx context.Context, opt *Options) *Client {
	// There is a little complexity here. The feature auto-refresh code
	// needs to keep track of some information about GrowthBook
	// instances to update them with new feature information, but we
	// want GrowthBook client instances to be garbage collected
	// normally. Go doesn't have weak references (if it did, the
	// auto-refresh code could keep weak references to GrowthBook client
	// instances), so we have to do something sneaky.
	//
	// The main GrowthBook client instance wraps a featureData instance
	// which contains the feature information. The auto-refresh code
	// stores references only to the inner featureData data structures.
	// The main outer Client data structure has a finalizer that handles
	// removing the inner featureData instance from the auto-refresh
	// code.
	//
	// This means that the lifecycle of the relevant objects is:
	//
	//  1. featureData value created (here in NewClient).
	//  2. Client value created, wrapping featureData value (here in
	//     NewClient).
	//  3. GrowthBook client instance is used...

	//  4. GrowthBook client instance is subscribed to auto-refresh
	//     updates. This adds a reference to the inner featureData value
	//     to the auto-refresh code's data structures.
	//
	//  ... more use of GrowthBook client instance ...
	//
	//  5. GrowthBook client instance is unreferenced, so eligible for
	//     GC.
	//  6. Garbage collection.
	//  7. GrowthBook client instance is collected by GC and its
	//     finalizer is run, which calls repoUnsubscribe. This removes
	//     the inner featureData instance from the auto-refresh code's
	//     data structures. (The finalizer resurrects the GrowthBook
	//     client instance, so another cycle of GC is needed to collect
	//     it for real.)
	//  8. Both the main GrowthBook client instance and the inner
	//     featureData instance are now unreferenced, so eligible for
	//     GC.
	//  9. Garbage collection.
	// 10. Main GrowthBook client instance and inner featureData
	//     instance are collected.
	//
	// The end result of all this is that the auto-refresh code can keep
	// hold of the data that it needs to update instances with new
	// features, but those resources are freed correctly when users drop
	// references to instances of the public Client structure.

	if opt == nil {
		opt = &Options{}
	}
	opt = opt.clone()
	opt.defaults()

	// Feature tracking information.
	features := &featureData{
		features:      FeatureMap{},
		decryptionKey: opt.DecryptionKey,
		ready:         false,
	}

	c := &Client{
		opt:                 opt,
		forcedVariations:    ForcedVariationsMap{},
		overrides:           ExperimentOverrides{},
		forcedFeatureValues: make(map[string]interface{}),
		attributeOverrides:  make(Attributes),
		trackedFeatures:     make(map[string]interface{}),
		trackedExperiments:  make(map[string]bool),
		subscriptions:       make(map[subscriptionID]ExperimentCallback),
		assigned:            make(assignments),
		features:            features,
	}

	runtime.SetFinalizer(c, func(c *Client) { repoUnsubscribe(c) })
	if opt.ClientKey != "" {
		go c.refresh(ctx, nil, true, false)
	}
	return c
}

func (c *Client) clone() *Client {
	c.features.RLock()
	defer c.features.RUnlock()

	return &Client{
		opt:                 c.opt.clone(),
		forcedVariations:    deepcopy.MustAnything(c.forcedVariations).(ForcedVariationsMap),
		overrides:           c.overrides.clone(),
		forcedFeatureValues: deepcopy.MustAnything(c.forcedFeatureValues).(map[string]interface{}),
		attributeOverrides:  deepcopy.MustAnything(c.attributeOverrides).(Attributes),
		trackedFeatures:     deepcopy.MustAnything(c.trackedFeatures).(map[string]interface{}),
		trackedExperiments:  deepcopy.MustAnything(c.trackedExperiments).(map[string]bool),
		subscriptions:       c.subscriptions.clone(),
		assigned:            c.assigned.clone(),
		features:            c.features.clone(),
	}
}

// Ready returns the ready flag, which indicates that features have
// been loaded.
func (c *Client) Ready() bool {
	return c.features.ready
}

// ForcedFeatures returns the current forced feature values.
func (c *Client) ForcedFeatures() map[string]interface{} {
	return c.forcedFeatureValues
}

// WithForcedFeatures updates the current forced feature values.
func (c *Client) WithForcedFeatures(values map[string]interface{}) *Client {
	if values == nil {
		values = map[string]interface{}{}
	}
	newc := c.clone()
	newc.forcedFeatureValues = values
	return newc
}

// AttributeOverrides returns the current attribute overrides.
func (c *Client) AttributeOverrides() Attributes {
	return c.attributeOverrides
}

// WithAttributeOverrides updates the current attribute overrides.
func (c *Client) WithAttributeOverrides(overrides Attributes) *Client {
	if overrides == nil {
		overrides = Attributes{}
	}
	newc := c.clone()
	newc.attributeOverrides = overrides.fixSliceTypes()
	return newc
}

// Overrides returns the current experiment overrides.
func (c *Client) Overrides() ExperimentOverrides {
	return c.overrides
}

// WithOverrides sets the current experiment overrides.
func (c *Client) WithOverrides(overrides ExperimentOverrides) *Client {
	if overrides == nil {
		overrides = ExperimentOverrides{}
	}
	newc := c.clone()
	newc.overrides = overrides
	return newc
}

// WithFeatures explicitly updates the current features.
func (c *Client) WithFeatures(features FeatureMap) *Client {
	newc := c.clone()
	newc.features.withFeatures(features)
	return newc
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

// Features returns the current features for a GrowthBook instance.
func (c *Client) Features() FeatureMap {
	return c.features.getFeatures()
}

func (feats *featureData) getFeatures() FeatureMap {
	feats.RLock()
	defer feats.RUnlock()
	return feats.features
}

// WithEncryptedFeatures updates the features in a GrowthBook instance
// from encrypted data.
func (c *Client) WithEncryptedFeatures(encrypted string, key string) (*Client, error) {
	newc := c.clone()
	err := newc.features.withEncryptedFeatures(encrypted, key)
	return newc, err
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
		err = json.Unmarshal([]byte(featuresJson), &features)
		if err == nil {
			feats.features = features
			feats.ready = true
		}
	}
	if err != nil || features == nil {
		err = errors.New("failed to decode encrypted features")
	}
	return err
}

// WithForcedVariations sets the forced variations in a GrowthBook
// instance.
func (c *Client) WithForcedVariations(forcedVariations ForcedVariationsMap) *Client {
	if forcedVariations == nil {
		forcedVariations = ForcedVariationsMap{}
	}
	newc := c.clone()
	newc.forcedVariations = forcedVariations
	return newc
}

func (c *Client) ForceVariation(key string, variation int) {
	c.ForceVariation(key, variation)
}

func (c *Client) UnforceVariation(key string) {
	c.UnforceVariation(key)
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
func (c *Client) IsOn(key string, attrs Attributes) (bool, error) {
	result, err := c.EvalFeature(key, attrs)
	if err != nil {
		return false, err
	}
	return result.On, nil
}

// IsOff determines whether a feature is off.
func (c *Client) IsOff(key string, attrs Attributes) (bool, error) {
	result, err := c.EvalFeature(key, attrs)
	if err != nil {
		return false, err
	}
	return result.Off, nil
}

// GetFeatureValue returns the result for a feature identified by a
// string feature key, with an explicit default.
func (c *Client) GetFeatureValue(key string, attrs Attributes,
	defaultValue interface{}) (interface{}, error) {
	featureValue, err := c.EvalFeature(key, attrs)
	if err != nil {
		return nil, err
	}
	if featureValue.Value != nil {
		return featureValue.Value, nil
	}
	return defaultValue, nil
}

// EffectiveAttributes returns the attributes in a GrowthBook's
// context, possibly modified by overrides.
func (c *Client) EffectiveAttributes(attrs Attributes) Attributes {
	c.features.RLock()
	defer c.features.RUnlock()

	effAttrs := Attributes{}
	for id, v := range attrs {
		effAttrs[id] = v
	}
	if c.attributeOverrides != nil {
		for id, v := range c.attributeOverrides {
			effAttrs[id] = v
		}
	}
	return effAttrs
}

// EvalFeature returns the result for a feature identified by a string
// feature key.
func (c *Client) EvalFeature(id string, attrs Attributes) (*FeatureResult, error) {
	c.features.RLock()
	defer c.features.RUnlock()

	attrs = attrs.fixSliceTypes()

	// Global override.
	if len(c.forcedFeatureValues) != 0 {
		if override, ok := c.forcedFeatureValues[id]; ok {
			logInfo(FeatureGlobalOverride, LogData{"id": id, "override": override})
			return c.getFeatureResult(id, override, OverrideResultSource, "", nil, nil)
		}
	}

	// Handle unknown features.
	feature, ok := c.features.features[id]
	if !ok {
		logWarn(FeatureUnknown, LogData{"feature": id})
		return c.getFeatureResult(id, nil, UnknownResultSource, "", nil, nil)
	}

	// Loop through the feature rules.
	for _, rule := range feature.Rules {
		// If the rule has a condition and the condition does not pass,
		// skip this rule.
		if rule.Condition != nil && !rule.Condition.Eval(c.EffectiveAttributes(attrs)) {
			logInfo(FeatureSkipCondition, LogData{"id": id, "rule": JSONLog{rule}})
			continue
		}

		// Apply any filters for who is included (e.g. namespaces).
		if rule.Filters != nil && c.isFilteredOut(rule.Filters, attrs) {
			logInfo(FeatureSkipFilters, LogData{"id": id, "rule": JSONLog{rule}})
			continue
		}

		// Feature value is being forced.
		if rule.Force != nil {
			// If this is a percentage rollout, skip if not included.
			seed := id
			if rule.Seed != "" {
				seed = rule.Seed
			}
			if !c.isIncludedInRollout(
				seed,
				rule.HashAttribute,
				attrs,
				rule.Range,
				rule.Coverage,
				rule.HashVersion,
			) {
				logInfo(FeatureSkipUserRollout, LogData{"id": id, "rule": JSONLog{rule}})
				continue
			}

			// Return forced feature result.
			logInfo(FeatureForceFromRule, LogData{"id": id, "rule": JSONLog{rule}})
			return c.getFeatureResult(id, rule.Force, ForceResultSource, rule.ID, nil, nil)
		}

		if rule.Variations == nil {
			logWarn(FeatureSkipInvalidRule, LogData{"id": id, "rule": JSONLog{rule}})
			continue
		}

		// Otherwise, convert the rule to an Experiment object, copying
		// values from the rule as necessary.
		exp := experimentFromFeatureRule(id, rule)

		// Run the experiment.
		result, err := c.doRun(exp, id, attrs)
		if err != nil {

		}
		c.fireSubscriptions(exp, result)

		// Only return a value if the user is part of the experiment.
		// c.fireSubscriptions(experiment, result)
		if result.InExperiment && !result.Passthrough {
			return c.getFeatureResult(id, result.Value, ExperimentResultSource, rule.ID, exp, result)
		}
	}

	// Fall back to using the default value.
	logInfo(FeatureUseDefaultValue, LogData{"id": id, "value": JSONLog{feature.DefaultValue}})
	return c.getFeatureResult(id, feature.DefaultValue, DefaultValueResultSource, "", nil, nil)
}

// Run an experiment. (Uses doRun to make wrapping for subscriptions
// simple.)
func (c *Client) Run(exp *Experiment, attrs Attributes) (*Result, error) {
	c.features.RLock()
	defer c.features.RUnlock()

	result, err := c.doRun(exp, "", attrs)
	if err != nil {
		return nil, err
	}
	c.fireSubscriptions(exp, result)
	return result, nil
}

// Subscribe adds a callback that is called every time GrowthBook.Run
// is called. This is different from the tracking callback since it
// also fires when a user is not included in an experiment.
func (c *Client) Subscribe(callback ExperimentCallback) func() {
	c.features.Lock()
	defer c.features.Unlock()

	id := c.nextSubscriptionID
	c.subscriptions[id] = callback
	c.nextSubscriptionID++
	return func() {
		delete(c.subscriptions, id)
	}
}

// GetAllResults returns a map containing all the latest results from
// all experiments that have been run, indexed by the experiment key.
func (c *Client) GetAllResults() map[string]*Assignment {
	return c.assigned
}

// ClearSavedResults clears out any experiment results saved within a
// GrowthBook instance (used for deciding whether to send data to
// subscriptions).
func (c *Client) ClearSavedResults() {
	c.assigned = make(map[string]*Assignment)
}

// ClearTrackingData clears out records of calls to the experiment
// tracking callback.
func (c *Client) ClearTrackingData() {
	// TODO: THREAD-SAFETY!
	c.trackedExperiments = make(map[string]bool)
}

// GetAPIInfo gets the hostname and client key for GrowthBook API
// access.
func (c *Client) GetAPIInfo() (string, string) {
	return strings.TrimRight(c.opt.APIHost, "/"), c.opt.ClientKey
}

type FeatureRepoOptions struct {
	AutoRefresh bool
	Timeout     time.Duration
	SkipCache   bool
}

func (c *Client) LoadFeatures(options *FeatureRepoOptions) error {
	return c.LoadFeaturesContext(context.Background(), options)
}

func (c *Client) LoadFeaturesContext(ctx context.Context, options *FeatureRepoOptions) error {
	err := c.refresh(ctx, options, true, true)
	if err != nil {
		return err
	}
	if options != nil && options.AutoRefresh {
		repoSubscribe(c)
	}
	return nil
}

func (c *Client) LatestFeatureUpdate() *time.Time {
	return repoLatestUpdate(c)
}

func (c *Client) RefreshFeatures(options *FeatureRepoOptions) error {
	return c.RefreshFeaturesContext(context.Background(), options)
}

func (c *Client) RefreshFeaturesContext(ctx context.Context, options *FeatureRepoOptions) error {
	return c.refresh(ctx, options, false, true)
}

//-- PRIVATE FUNCTIONS START HERE ----------------------------------------------

func (c *Client) refresh(
	ctx context.Context,
	options *FeatureRepoOptions, allowStale bool, updateInstance bool) error {

	if c.opt.ClientKey == "" {
		return errors.New("Missing clientKey")
	}
	var timeout time.Duration
	skipCache := c.opt.DevMode
	if options != nil {
		timeout = options.Timeout
		skipCache = skipCache || options.SkipCache
	}
	return repoRefreshFeatures(ctx, c, timeout, skipCache, allowStale, updateInstance)
}

func (c *Client) trackFeatureUsage(key string, res *FeatureResult) {
	// Don't track feature usage that was forced via an override.
	if res.Source == OverrideResultSource {
		return
	}

	// Only track a feature once, unless the assigned value changed.
	if saved, ok := c.trackedFeatures[key]; ok && reflect.DeepEqual(saved, res.Value) {
		return
	}
	c.trackedFeatures[key] = res.Value

	// Fire user-supplied callback
	if c.opt.OnFeatureUsage != nil {
		c.opt.OnFeatureUsage(key, res)
	}
}

func (c *Client) getFeatureResult(
	key string,
	value FeatureValue,
	source FeatureResultSource,
	ruleID string,
	experiment *Experiment,
	result *Result) (*FeatureResult, error) {
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

	c.trackFeatureUsage(key, &retval)

	return &retval, nil
}

func (c *Client) getResult(
	exp *Experiment, attrs Attributes, variationIndex int,
	hashUsed bool, featureID string, bucket *float64) (*Result, error) {
	inExperiment := true

	// If assigned variation is not valid, use the baseline and mark the
	// user as not in the experiment
	if variationIndex < 0 || variationIndex >= len(exp.Variations) {
		variationIndex = 0
		inExperiment = false
	}

	// Get the hashAttribute and hashValue
	hashAttribute, hashString := c.getHashAttribute(exp.HashAttribute, attrs)

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
	}, nil
}

func (c *Client) fireSubscriptions(exp *Experiment, result *Result) {
	// Determine whether the result changed from the last stored result
	// for the experiment.
	changed := false
	storedResult, exists := c.assigned[exp.Key]
	if exists {
		if storedResult.Result.InExperiment != result.InExperiment ||
			storedResult.Result.VariationID != result.VariationID {
			changed = true
		}
	}

	// Store the experiment result.
	c.assigned[exp.Key] = &Assignment{exp, result}

	// If the result changed, trigger all subscriptions.
	if changed || !exists {
		for _, sub := range c.subscriptions {
			sub(exp, result)
		}
	}
}

// Worker function to run an experiment.
func (c *Client) doRun(exp *Experiment, featureID string, attrs Attributes) (*Result, error) {
	// 1. If experiment has fewer than two variations, return default
	//    result.
	if len(exp.Variations) < 2 {
		logWarn(ExperimentInvalid, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 2. If the client is disabled, return default result.
	if c.opt.Disabled {
		logInfo(ExperimentDisabled, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 2.5. Merge in experiment overrides from the context
	exp = c.mergeOverrides(exp)

	// 3. If a variation is forced from a querystring, return the forced
	//    variation.
	if c.opt.URL != nil {
		qsOverride := getQueryStringOverride(exp.Key, c.opt.URL, len(exp.Variations))
		if qsOverride != nil {
			logInfo(ExperimentForceViaQueryString, LogData{"key": exp.Key, "qsOverride": *qsOverride})
			return c.getResult(exp, attrs, *qsOverride, false, featureID, nil)
		}
	}

	// 4. If a variation is forced in the context, return the forced
	//    variation.
	if c.forcedVariations != nil {
		force, forced := c.forcedVariations[exp.Key]
		if forced {
			logInfo(ExperimentForcedVariation, LogData{"key": exp.Key, "force": force})
			return c.getResult(exp, attrs, force, false, featureID, nil)
		}
	}

	// 5. Exclude inactive experiments and return default result.
	if exp.Status == DraftStatus || !exp.Active {
		logInfo(ExperimentSkipInactive, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 6. Get the user hash value and return if empty.
	_, hashString := c.getHashAttribute(exp.HashAttribute, attrs)
	if hashString == "" {
		logInfo(ExperimentSkipMissingHashAttribute, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 7. If exp.Namespace is set, return if not in range.
	if exp.Filters != nil {
		if c.isFilteredOut(exp.Filters, attrs) {
			logInfo(ExperimentSkipFilters, LogData{"key": exp.Key})
			return c.getResult(exp, attrs, -1, false, featureID, nil)
		}
	} else if exp.Namespace != nil {
		if !exp.Namespace.inNamespace(hashString) {
			logInfo(ExperimentSkipNamespace, LogData{"key": exp.Key})
			return c.getResult(exp, attrs, -1, false, featureID, nil)
		}
	}

	// 7.5. Exclude if include function returns false.
	if exp.Include != nil && !exp.Include() {
		logInfo(ExperimentSkipIncludeFunction, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 8. Exclude if condition is false.
	if exp.Condition != nil {
		if !exp.Condition.Eval(c.EffectiveAttributes(attrs)) {
			logInfo(ExperimentSkipCondition, LogData{"key": exp.Key})
			return c.getResult(exp, attrs, -1, false, featureID, nil)
		}
	}

	// 8.1. Exclude if user is not in a required group.
	if exp.Groups != nil && !c.hasGroupOverlap(exp.Groups) {
		logInfo(ExperimentSkipGroups, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 8.2. Old style URL targeting.
	if exp.URL != nil && !c.urlIsValid(exp.URL) {
		logInfo(ExperimentSkipURL, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 8.3. New, more powerful URL targeting
	if exp.URLPatterns != nil {
		targeted, err := isURLTargeted(c.opt.URL, exp.URLPatterns)
		if err != nil {
			return nil, err
		}
		if !targeted {
			logInfo(ExperimentSkipURLTargeting, LogData{"key": exp.Key})
			return c.getResult(exp, attrs, -1, false, featureID, nil)
		}
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
		logWarn(ExperimentSkipInvalidHashVersion, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
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
		logInfo(ExperimentSkipCoverage, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 11. If experiment has a forced variation, return it.
	if exp.Force != nil {
		return c.getResult(exp, attrs, *exp.Force, false, featureID, nil)
	}

	// 12. If in QA mode, return default result.
	if c.opt.QAMode {
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 12.5. Exclude if experiment is stopped.
	if exp.Status == StoppedStatus {
		logInfo(ExperimentSkipStopped, LogData{"key": exp.Key})
		return c.getResult(exp, attrs, -1, false, featureID, nil)
	}

	// 13. Build the result object.
	result, err := c.getResult(exp, attrs, assigned, true, featureID, n)

	// 14. Fire tracking callback if required.
	if err == nil {
		c.track(exp, result)
	}

	// InExperiment
	logInfo(InExperiment, LogData{"key": exp.Key, "variationID": result.VariationID})
	return result, err
}

func (c *Client) mergeOverrides(exp *Experiment) *Experiment {
	if c.overrides == nil {
		return exp
	}
	if override, ok := c.overrides[exp.Key]; ok {
		exp = exp.applyOverride(override)
	}
	return exp
}

// Fire Context.TrackingCallback if it's set and the combination of
// hashAttribute, hashValue, experiment key, and variation ID has not
// been tracked before.
func (c *Client) track(exp *Experiment, result *Result) {
	if c.opt.TrackingCallback == nil {
		return
	}

	// Make sure tracking callback is only fired once per unique
	// experiment.
	key := fmt.Sprintf("%s%v%s%d", result.HashAttribute, result.HashValue,
		exp.Key, result.VariationID)
	if _, exists := c.trackedExperiments[key]; exists {
		return
	}

	c.trackedExperiments[key] = true
	c.opt.TrackingCallback(exp, result)
}

func (c *Client) getHashAttribute(attr string, attrs Attributes) (string, string) {
	hashAttribute := "id"
	if attr != "" {
		hashAttribute = attr
	}

	var hashValue interface{}
	ok := false
	if c.attributeOverrides != nil {
		hashValue, ok = c.attributeOverrides[hashAttribute]
	}
	if !ok {
		if attrs != nil {
			hashValue, ok = attrs[hashAttribute]
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

func (c *Client) isIncludedInRollout(
	seed string,
	hashAttribute string,
	attrs Attributes,
	rng *Range,
	coverage *float64,
	hashVersion int,
) bool {
	if rng == nil && coverage == nil {
		return true
	}

	_, hashValue := c.getHashAttribute(hashAttribute, attrs)
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

func (c *Client) isFilteredOut(filters []Filter, attrs Attributes) bool {
	for _, filter := range filters {
		_, hashValue := c.getHashAttribute(filter.Attribute, attrs)
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

func (c *Client) hasGroupOverlap(groups []string) bool {
	for _, g := range groups {
		if val, ok := c.opt.Groups[g]; ok && val {
			return true
		}
	}
	return false
}

func (c *Client) urlIsValid(urlRegexp *regexp.Regexp) bool {
	regurl := c.opt.URL
	if regurl == nil {
		return false
	}

	return urlRegexp.MatchString(regurl.String()) ||
		urlRegexp.MatchString(regurl.Path)
}
