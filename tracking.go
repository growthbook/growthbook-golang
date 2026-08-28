package growthbook

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

// TrackingUserContext is the user context an evaluation ran with — the
// evaluating client's attributes and URL. It is passed to ExperimentCallback
// and carried on TrackingData, mirroring the JS SDK type of the same name.
type TrackingUserContext struct {
	Attributes Attributes `json:"attributes"`
	URL        string     `json:"url,omitempty"`
}

// TrackingData is a single experiment exposure, in the JSON shape client
// SDKs accept as deferred tracking calls.
type TrackingData struct {
	Experiment *Experiment          `json:"experiment"`
	Result     *ExperimentResult    `json:"result"`
	User       *TrackingUserContext `json:"user,omitempty"`
}

// DedupeKey identifies an exposure by hash attribute, hash value, experiment
// key, and variation. Exposures are deduplicated by this key within a single
// evaluation and in the deferred tracking buffer.
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

func (client *Client) trackingUserContext() *TrackingUserContext {
	clientURL := ""
	if client.url != nil {
		clientURL = client.url.String()
	}
	return &TrackingUserContext{
		Attributes: attributesFromValue(client.attributes),
		URL:        clientURL,
	}
}

func (e *evaluator) recordExperiment(exp *Experiment, res *ExperimentResult) {
	if !e.recording {
		return
	}
	if e.userCtx == nil {
		e.userCtx = e.client.trackingUserContext()
	}
	data := TrackingData{Experiment: exp, Result: res, User: e.userCtx}
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

// recordFeatureUsage reports a feature once per evaluation unless its value
// changed.
func (e *evaluator) recordFeatureUsage(key string, res *FeatureResult) {
	if !e.recording {
		return
	}
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
