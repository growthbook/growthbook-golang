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
// SDKs accept as deferred tracking calls. It holds its own copies of the
// experiment and result, snapshotted at evaluation time.
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
// in first-seen order — as detached copies in the SDK's JSON shape, safe to
// retain or mutate without affecting the buffer. Returns nil unless deferred
// tracking is enabled (see WithDeferredTracking).
func (client *Client) DeferredTrackingCalls() []TrackingData {
	if client.deferredTracks == nil {
		return nil
	}
	b := client.deferredTracks
	b.mu.Lock()
	shared := append([]TrackingData(nil), b.data...)
	b.mu.Unlock()

	detached, err := detachTrackingData(shared)
	if err != nil {
		client.logger.Warn("Returning aliased tracking data, deep copy failed", "error", err)
		return shared
	}
	return detached
}

func detachTrackingData(data []TrackingData) ([]TrackingData, error) {
	if len(data) == 0 {
		return data, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var out []TrackingData
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
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
	key := TrackingData{Experiment: exp, Result: res}.DedupeKey()
	if e.trackedExperiments[key] {
		return
	}
	if e.trackedExperiments == nil {
		e.trackedExperiments = make(map[string]bool)
	}
	e.trackedExperiments[key] = true
	if e.userCtx == nil {
		e.userCtx = e.client.trackingUserContext()
	}
	expCopy, resCopy := *exp, *res
	e.experiments = append(e.experiments, TrackingData{Experiment: &expCopy, Result: &resCopy, User: e.userCtx})
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
