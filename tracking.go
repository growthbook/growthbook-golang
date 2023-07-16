package growthbook

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// ExperimentTracker is an interface with a callback method that is
// executed every time a user is included in an Experiment. It is also
// the type used for subscription functions, which are called whenever
// Experiment.Run is called and the experiment result changes,
// independent of whether a user is inncluded in the experiment or
// not.

type ExperimentTracker interface {
	Track(ctx context.Context, c *Client,
		exp *Experiment, result *Result, extraData interface{})
}

// ExperimentCallback is a wrapper around a simple callback for
// experiment tracking.

type ExperimentCallback struct {
	CB func(ctx context.Context, exp *Experiment, result *Result)
}

func (tcb *ExperimentCallback) Track(ctx context.Context,
	c *Client, exp *Experiment, result *Result, extraData interface{}) {
	tcb.CB(ctx, exp, result)
}

// FeatureUsageTracker is an interface with a callback method that is
// executed every time a feature is evaluated.

type FeatureUsageTracker interface {
	OnFeatureUsage(ctx context.Context, c *Client,
		key string, result *FeatureResult, extraData interface{})
}

// FeatureUsageCallback is a wrapper around a simple callback for
// feature usage tracking.

type FeatureUsageCallback struct {
	CB func(ctx context.Context, key string, result *FeatureResult)
}

func (fcb *FeatureUsageCallback) OnFeatureUsage(ctx context.Context,
	c *Client, key string, result *FeatureResult, extraData interface{}) {
	fcb.CB(ctx, key, result)
}

// ExperimentTrackingCache is an interface to a cache used for holding
// records of calls to the experiment tracker. A simple default
// implementation is provided, but for long-lived server processes, a
// LRU cache might be more appropriate; or for horizontal scaling
// cases, a distributed cache using memcached or Redis might be
// better. Both cases can be handled by implementing this interface.

type ExperimentTrackingCache interface {
	// Return true if the experiment tracker should be called, and cache
	// whatever data is required to suppress subsequent unneeded calls
	// to the tracker.
	Check(ctx context.Context, c *Client,
		exp *Experiment, result *Result, extraData interface{}) bool

	// Clear the tracking cache.
	Clear()
}

// The default experiment tracking cache used if the user doesn't
// supply one of their own. Simple thread-safe "never evict" cache.

type defaultExperimentTrackingCache struct {
	sync.Mutex
	trackedExperiments map[string]struct{}
}

func newDefaultExperimentTrackingCache() *defaultExperimentTrackingCache {
	return &defaultExperimentTrackingCache{
		trackedExperiments: make(map[string]struct{}),
	}
}

func (cache *defaultExperimentTrackingCache) Check(ctx context.Context, c *Client,
	exp *Experiment, result *Result, extraData interface{}) bool {
	cache.Lock()
	defer cache.Unlock()

	// If the experiment already exists in the cache, we don't need to
	// call the tracker.
	key := fmt.Sprintf("%s%v%s%d", result.HashAttribute, result.HashValue,
		exp.Key, result.VariationID)
	if _, exists := cache.trackedExperiments[key]; exists {
		return false
	}

	// Add the experiment to the cache and mark that the tracker should
	// be called.
	cache.trackedExperiments[key] = struct{}{}
	return true
}

func (cache *defaultExperimentTrackingCache) Clear() {
	cache.Lock()
	defer cache.Unlock()

	cache.trackedExperiments = make(map[string]struct{})
}

// FeatureUsageTrackingCache is an interface to a cache used for
// holding records of calls to the feature usage tracker. A simple
// default implementation is provided, but for long-lived server
// processes, a LRU cache might be more appropriate; or for horizontal
// scaling cases, a distributed cache using memcached or Redis might
// be better. Both cases can be handled by implementing this
// interface.

type FeatureUsageTrackingCache interface {
	// Return true if the feature usage tracker should be called, and
	// cache whatever data is required to suppress subsequent unneeded
	// calls to the tracker.
	Check(ctx context.Context, c *Client,
		key string, res *FeatureResult, extraData interface{}) bool

	// Clear the tracking cache.
	Clear()
}

// The default experiment tracking cache used if the user doesn't
// supply one of their own. Simple thread-safe "never evict" cache.

type defaultFeatureUsageTrackingCache struct {
	sync.Mutex
	trackedFeatures map[string]interface{}
}

func newDefaultFeatureUsageTrackingCache() *defaultFeatureUsageTrackingCache {
	return &defaultFeatureUsageTrackingCache{
		trackedFeatures: make(map[string]interface{}),
	}
}

func (cache *defaultFeatureUsageTrackingCache) Check(ctx context.Context, c *Client,
	key string, res *FeatureResult, extraData interface{}) bool {
	cache.Lock()
	defer cache.Unlock()

	// Only track a feature once, unless the assigned value changed.
	if saved, ok := cache.trackedFeatures[key]; ok && reflect.DeepEqual(saved, res.Value) {
		return false
	}

	// Add the feature value to the cache and mark that the tracker
	// should be called.
	cache.trackedFeatures[key] = res.Value
	return true
}

func (cache *defaultFeatureUsageTrackingCache) Clear() {
	cache.Lock()
	defer cache.Unlock()

	cache.trackedFeatures = make(map[string]interface{})
}
