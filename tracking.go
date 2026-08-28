package growthbook

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// TrackingData is a single experiment exposure: the experiment and the
// user's assignment. It matches the JS SDK's TrackingData shape, so a
// serialized list can be handed to a client-side GrowthBook instance via
// setDeferredTrackingCalls.
type TrackingData struct {
	Experiment *Experiment       `json:"experiment"`
	Result     *ExperimentResult `json:"result"`
}

// DedupeKey identifies an exposure the way the JS SDK's
// getExperimentDedupeKey does. Exposures are deduplicated by this key within
// a single evaluation; callers batching several evaluations for the same
// user can use it to deduplicate across them.
func (t TrackingData) DedupeKey() string {
	return t.Result.HashAttribute + t.Result.HashValue + t.Experiment.Key + strconv.Itoa(t.Result.VariationId)
}

// FeatureUsage is a single feature evaluation: the feature key and its
// result.
type FeatureUsage struct {
	Key    string         `json:"key"`
	Result *FeatureResult `json:"result"`
}

// EvalTracking is everything one evaluation reports through callbacks and
// plugins: every feature evaluated (the requested feature and any
// prerequisites) and every experiment assignment made along the way —
// including passthrough assignments — in evaluation order.
//
// It is scoped to a single EvalFeatureWithTracking /
// RunExperimentWithTracking call and complete when that call returns, so a
// server that forwards exposures elsewhere (e.g. remote evaluation) can read
// it synchronously instead of correlating the client-wide callbacks across
// concurrent requests.
type EvalTracking struct {
	FeatureUsage []FeatureUsage `json:"featureUsage"`
	Experiments  []TrackingData `json:"experiments"`
}

// recordExperiment saves an assignment for reporting. The JS SDK fires its
// tracking callback at the equivalent point in its runExperiment, deduping
// repeats of the same assignment.
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
	e.tracking.Experiments = append(e.tracking.Experiments, data)
}

// recordFeatureUsage saves a feature evaluation for reporting. Mirrors the
// JS SDK's onFeatureUsage: a feature is reported once per evaluation unless
// its value changed.
func (e *evaluator) recordFeatureUsage(key string, res *FeatureResult) {
	stringified := stringifyFeatureValue(res.Value)
	if prev, ok := e.trackedFeatures[key]; ok && prev == stringified {
		return
	}
	if e.trackedFeatures == nil {
		e.trackedFeatures = make(map[string]string)
	}
	e.trackedFeatures[key] = stringified
	e.tracking.FeatureUsage = append(e.tracking.FeatureUsage, FeatureUsage{Key: key, Result: res})
}

func stringifyFeatureValue(v FeatureValue) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
