package growthbook

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
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
// key, and variation.
//
// Deprecated: informational only. The SDK no longer dedupes on this string —
// field values containing the NUL delimiter can make two distinct exposures
// share a key. Internal deduplication uses a field-wise comparable key that
// cannot collide.
func (t TrackingData) DedupeKey() string {
	return t.Result.HashAttribute + "\x00" + t.Result.HashValue + "\x00" + t.Experiment.Key + "\x00" + strconv.Itoa(t.Result.VariationId)
}

// trackingKey is the exposure identity used for deduplication, within a
// single evaluation and across a tracking buffer's lifetime. A comparable
// struct rather than a joined string: no delimiter means no cross-field
// collisions when values contain the delimiter byte.
type trackingKey struct {
	hashAttribute string
	hashValue     string
	experimentKey string
	variationId   int
}

func dedupeKey(exp *Experiment, res *ExperimentResult) trackingKey {
	return trackingKey{
		hashAttribute: res.HashAttribute,
		hashValue:     res.HashValue,
		experimentKey: exp.Key,
		variationId:   res.VariationId,
	}
}

type featureUsage struct {
	key    string
	result *FeatureResult
}

// TrackingBuffer accumulates experiment exposures across evaluations,
// deduplicated for the buffer's lifetime, in first-seen order. Attach one to
// a client with WithTrackingBuffer; every client sharing the buffer (clones
// included) collects into it. Safe for concurrent use; the zero value is
// ready to use.
//
// A TrackingBuffer must not be copied after first use: a copy would share
// the buffered state while holding an independent mutex. Share it by
// pointer, as WithTrackingBuffer does (`go vet`'s copylocks check flags
// value copies).
type TrackingBuffer struct {
	mu   sync.Mutex
	seen map[trackingKey]bool
	data []TrackingData
}

// NewTrackingBuffer creates an empty tracking buffer.
func NewTrackingBuffer() *TrackingBuffer {
	return &TrackingBuffer{}
}

func (b *TrackingBuffer) add(data []TrackingData) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seen == nil {
		b.seen = make(map[trackingKey]bool)
	}
	for _, d := range data {
		key := dedupeKey(d.Experiment, d.Result)
		if b.seen[key] {
			continue
		}
		b.seen[key] = true
		b.data = append(b.data, d)
	}
}

// TrackingCalls returns the exposures buffered so far — passthrough and
// prerequisite assignments included, in first-seen order — as detached
// copies in the SDK's JSON shape, safe to retain or mutate without affecting
// the buffer. It does not drain the buffer; pair with Clear.
func (b *TrackingBuffer) TrackingCalls() []TrackingData {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	shared := append([]TrackingData(nil), b.data...)
	b.mu.Unlock()
	// Entries were proven serializable when buffered, so this detach cannot
	// drop any; a nil logger is fine.
	return detachTrackingData(shared, nil)
}

// TakeTrackingCalls atomically returns the buffered exposures and empties
// the buffer — the drain form of TrackingCalls followed by Clear. Unlike
// calling those two separately, an exposure recorded concurrently can never
// be cleared without having been returned, so a long-lived shared buffer can
// be drained in a loop without losing exposures. Like Clear, it forgets the
// dedupe memory: a later identical exposure is recorded again.
func (b *TrackingBuffer) TakeTrackingCalls() []TrackingData {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	taken := b.data
	b.seen = nil
	b.data = nil
	b.mu.Unlock()
	// Entries were proven serializable when buffered, so this detach cannot
	// drop any; a nil logger is fine.
	return detachTrackingData(taken, nil)
}

// Clear empties the buffer, including its dedupe memory: a subsequent
// exposure identical to one seen before Clear is recorded again. For the
// intended request-scoped lifecycle (one buffer per request, read once,
// then cleared or dropped) this never matters.
func (b *TrackingBuffer) Clear() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seen = nil
	b.data = nil
}

// DeferredTrackingCalls returns the exposures collected by the client's
// attached tracking buffer (see TrackingBuffer.TrackingCalls). Returns nil
// when no buffer is attached.
func (client *Client) DeferredTrackingCalls() []TrackingData {
	return client.trackingBuffer.TrackingCalls()
}

// detachTrackingData deep-copies entries via a JSON round-trip, dropping
// (and logging, when a logger is given) any entry that cannot be
// serialized — such an exposure could not be forwarded anyway, and
// returning it aliased would break the detached-copy guarantee.
func detachTrackingData(data []TrackingData, logger *slog.Logger) []TrackingData {
	if len(data) == 0 {
		return nil
	}
	out := make([]TrackingData, 0, len(data))
	for _, d := range data {
		raw, err := json.Marshal(d)
		if err == nil {
			var detached TrackingData
			if err = json.Unmarshal(raw, &detached); err == nil {
				out = append(out, detached)
				continue
			}
		}
		if logger != nil {
			logger.Warn("Dropping unserializable exposure from deferred tracking",
				"experiment", d.Experiment.Key, "error", err)
		}
	}
	return out
}

// TakeDeferredTrackingCalls atomically drains the client's attached tracking
// buffer (see TrackingBuffer.TakeTrackingCalls). Returns nil when no buffer
// is attached.
func (client *Client) TakeDeferredTrackingCalls() []TrackingData {
	return client.trackingBuffer.TakeTrackingCalls()
}

// ClearDeferredTrackingCalls empties the client's attached tracking buffer
// (see TrackingBuffer.Clear). No-op when no buffer is attached.
func (client *Client) ClearDeferredTrackingCalls() {
	client.trackingBuffer.Clear()
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
	if e.trackedExperiments == nil {
		e.trackedExperiments = make(map[trackingKey]bool)
	}
	key := dedupeKey(exp, res)
	if e.trackedExperiments[key] {
		return
	}
	e.trackedExperiments[key] = true
	if e.userCtx == nil {
		e.userCtx = e.client.trackingUserContext()
	}
	expCopy, resCopy := *exp, *res
	// Deep-clone what describes the assignment: subscribers and callbacks
	// receive the live experiment and result after this snapshot is taken,
	// and the buffer's JSON detach happens later at flush — shared mutables
	// would let third-party code alter recorded propensities or bucketing
	// semantics. (Other experiment fields — meta, filters, conditions —
	// remain shallow-copied, a pre-existing trade-off; they don't feed
	// bandit training.)
	expCopy.Weights = slices.Clone(expCopy.Weights)
	expCopy.Ranges = slices.Clone(expCopy.Ranges)
	if cb := expCopy.ContextualBandit; cb != nil {
		cbCopy := *cb
		cbCopy.VariationWeights = slices.Clone(cb.VariationWeights)
		cbCopy.BanditVersion = clonedBanditVersion(cb.BanditVersion)
		expCopy.ContextualBandit = &cbCopy
	}
	resCopy.VariationWeights = slices.Clone(resCopy.VariationWeights)
	resCopy.BanditVersion = clonedBanditVersion(resCopy.BanditVersion)
	e.experiments = append(e.experiments, TrackingData{Experiment: &expCopy, Result: &resCopy, User: e.userCtx})
}

// recordFeatureUsage reports a feature once per evaluation unless its value
// changed.
func (e *evaluator) recordFeatureUsage(key string, res *FeatureResult) {
	if !e.recording {
		return
	}
	if e.trackedFeatures == nil {
		e.trackedFeatures = make(map[string]string)
	}
	stringified := stringifyFeatureValue(res.Value)
	if prev, ok := e.trackedFeatures[key]; ok && prev == stringified {
		return
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
