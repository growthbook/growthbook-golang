package growthbook

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

// TrackingData is a single experiment exposure. It serializes to the JS
// SDK's TrackingData shape, compatible with setDeferredTrackingCalls.
type TrackingData struct {
	Experiment *Experiment       `json:"experiment"`
	Result     *ExperimentResult `json:"result"`
}

// DedupeKey identifies an exposure by the same fields as the JS SDK's
// getExperimentDedupeKey, with separators so distinct exposures can't
// collide on field boundaries. Exposures are deduplicated by this key within
// a single evaluation and in the deferred tracking buffer.
func (t TrackingData) DedupeKey() string {
	return t.Result.HashAttribute + "\x00" + t.Result.HashValue + "\x00" + t.Experiment.Key + "\x00" + strconv.Itoa(t.Result.VariationId)
}

type featureUsage struct {
	key    string
	result *FeatureResult
}

// trackingBuffer accumulates exposures across evaluations, deduped by
// DedupeKey, keeping first-seen order.
type trackingBuffer struct {
	mu   sync.Mutex
	seen map[string]bool
	data []TrackingData
}

func newTrackingBuffer() *trackingBuffer {
	return &trackingBuffer{seen: make(map[string]bool)}
}

func (b *trackingBuffer) add(data []TrackingData) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, d := range data {
		key := d.DedupeKey()
		if b.seen[key] {
			continue
		}
		b.seen[key] = true
		b.data = append(b.data, d)
	}
}

// DeferredTrackingCalls returns the experiment exposures buffered so far —
// passthrough and prerequisite assignments included, deduped by DedupeKey,
// in first-seen order. Returns nil unless deferred tracking is enabled (see
// WithDeferredTracking).
func (client *Client) DeferredTrackingCalls() []TrackingData {
	if client.deferredTracks == nil {
		return nil
	}
	b := client.deferredTracks
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]TrackingData(nil), b.data...)
}

// ClearDeferredTrackingCalls empties the deferred tracking buffer.
func (client *Client) ClearDeferredTrackingCalls() {
	if client.deferredTracks == nil {
		return
	}
	b := client.deferredTracks
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seen = make(map[string]bool)
	b.data = nil
}

func (e *evaluator) recordExperiment(exp *Experiment, res *ExperimentResult) {
	data := TrackingData{Experiment: exp, Result: res}
	key := data.DedupeKey()
	if e.trackedExperiments[key] {
		return
	}
	if e.trackedExperiments == nil {
		e.trackedExperiments = make(map[string]bool)
	}
	e.trackedExperiments[key] = true
	e.experiments = append(e.experiments, data)
}

// recordFeatureUsage mirrors the JS SDK's onFeatureUsage: a feature is
// reported once per evaluation unless its value changed.
func (e *evaluator) recordFeatureUsage(key string, res *FeatureResult) {
	stringified := stringifyFeatureValue(res.Value)
	if prev, ok := e.trackedFeatures[key]; ok && prev == stringified {
		return
	}
	if e.trackedFeatures == nil {
		e.trackedFeatures = make(map[string]string)
	}
	e.trackedFeatures[key] = stringified
	e.featureUsage = append(e.featureUsage, featureUsage{key: key, result: res})
}

func stringifyFeatureValue(v FeatureValue) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
