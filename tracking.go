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
// Experiment.Run is called, independent of whether a user is
// inncluded in the experiment or not.
//
// The tracker itself is responsible for caching behaviour and
// preventing repeated tracking of experiments. A simple no-delete
// cache suitable for debug use in a single process is provided (see
// SingleProcessExperimentTrackingCache).

type ExperimentTracker interface {
	Track(ctx context.Context, c *Client,
		exp *Experiment, result *Result, extraData any)
}

// ExperimentCallback is a wrapper around a simple callback for
// experiment tracking.

type ExperimentCallback struct {
	CB func(ctx context.Context, exp *Experiment, result *Result)
}

func (tcb *ExperimentCallback) Track(ctx context.Context,
	c *Client, exp *Experiment, result *Result, extraData any) {
	tcb.CB(ctx, exp, result)
}

// A simple single process experiment tracking cache that can be used
// if the user doesn't supply one of their own. Simple thread-safe
// "never evict" cache.
//
// Generally, caching machinery should just wrap an ExperimentTracker
// value, or implement the tracking interface itself directly. A
// simple default implementation is provided as
// SingleProcessExperimentTrackingCache, but for long-lived server
// processes, a LRU cache might be more appropriate; or for horizontal
// scaling cases, a distributed cache using memcached or Redis might
// be better. Both cases can be handled by implementing the
// ExperimentTracker interface.
//
// A typical and simplest use might be (`callback` is a callback
// function):
//
// 	  tracker := NewSingleProcessExperimentTrackingCache(
// 	    &ExperimentCallback{callback},
// 	  )
// 	  client := NewClient(&Options{ExperimentTracker: tracker})

type SingleProcessExperimentTrackingCache struct {
	sync.Mutex
	trackedExperiments map[string]struct{}
	tracker            ExperimentTracker
}

func NewSingleProcessExperimentTrackingCache(tracker ExperimentTracker) *SingleProcessExperimentTrackingCache {
	return &SingleProcessExperimentTrackingCache{
		trackedExperiments: make(map[string]struct{}),
		tracker:            tracker,
	}
}

// Implement the ExperimentTracker interface, wrapping the inner
// tracker.

func (cache *SingleProcessExperimentTrackingCache) Track(ctx context.Context, c *Client,
	exp *Experiment, result *Result, extraData any) {
	cache.Lock()
	defer cache.Unlock()

	// If the experiment already exists in the cache, we don't need to
	// call the tracker.
	key := fmt.Sprintf("%s%v%s%d", result.HashAttribute, result.HashValue,
		exp.Key, result.VariationID)
	if _, exists := cache.trackedExperiments[key]; exists {
		return
	}

	// Add the experiment to the cache and call the tracker.
	cache.trackedExperiments[key] = struct{}{}
	cache.tracker.Track(ctx, c, exp, result, extraData)
}

func (cache *SingleProcessExperimentTrackingCache) Clear() {
	cache.Lock()
	defer cache.Unlock()

	cache.trackedExperiments = make(map[string]struct{})
}

// FeatureUsageTracker is an interface with a callback method that is
// executed every time a feature is evaluated.
//
// The tracker itself is responsible for caching behaviour and
// preventing repeated tracking of feature usage if this isn't wanted.
// A simple no-delete cache suitable for debug use in a single process
// is provided (see SingleProcessFeatureUsageTrackingCache).

type FeatureUsageTracker interface {
	OnFeatureUsage(ctx context.Context, c *Client,
		key string, result *FeatureResult, extraData any)
}

// FeatureUsageCallback is a wrapper around a simple callback for
// feature usage tracking.

type FeatureUsageCallback struct {
	CB func(ctx context.Context, key string, result *FeatureResult)
}

func (fcb *FeatureUsageCallback) OnFeatureUsage(ctx context.Context,
	c *Client, key string, result *FeatureResult, extraData any) {
	fcb.CB(ctx, key, result)
}

// A simple single process feature usage tracking cache that can be
// used if the user doesn't supply one of their own. Simple
// thread-safe "never evict" cache.
//
// Generally, caching machinery should just wrap a FeatureUsageTracker
// value, or implement the tracking interface itself directly. A
// simple default implementation is provided as
// SingleProcessFeatureUsageTrackingCache, but for long-lived server
// processes, a LRU cache might be more appropriate; or for horizontal
// scaling cases, a distributed cache using memcached or Redis might
// be better. Both cases can be handled by implementing the
// ExperimentTracker interface.
//
// A typical and simplest use might be (`callback` is a callback
// function):
//
// 	  tracker := NewSingleProcessFeatureUsageTrackingCache(
// 	    &FeatureUsageCallback{callback},
// 	  )
// 	  client := NewClient(&Options{FeatureUsageTracker: tracker})

type SingleProcessFeatureUsageTrackingCache struct {
	sync.Mutex
	trackedFeatures map[string]any
	tracker         FeatureUsageTracker
}

func NewSingleProcessFeatureUsageTrackingCache(tracker FeatureUsageTracker) *SingleProcessFeatureUsageTrackingCache {
	return &SingleProcessFeatureUsageTrackingCache{
		trackedFeatures: make(map[string]any),
		tracker:         tracker,
	}
}

// Implement the feature usage tracking interface.

func (cache *SingleProcessFeatureUsageTrackingCache) OnFeatureUsage(ctx context.Context, c *Client,
	key string, res *FeatureResult, extraData any) {
	cache.Lock()
	defer cache.Unlock()

	// Only track a feature once, unless the assigned value changed.
	if saved, ok := cache.trackedFeatures[key]; ok && reflect.DeepEqual(saved, res.Value) {
		return
	}

	// Add the feature value to the cache and call the tracker.
	cache.trackedFeatures[key] = res.Value
	cache.tracker.OnFeatureUsage(ctx, c, key, res, extraData)
}

func (cache *SingleProcessFeatureUsageTrackingCache) Clear() {
	cache.Lock()
	defer cache.Unlock()

	cache.trackedFeatures = make(map[string]any)
}
