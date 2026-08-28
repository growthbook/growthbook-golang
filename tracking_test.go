package growthbook

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Weights of [0,1] or [1,0] make every assignment deterministic. The
// "ramped" features model a monitored ramp step, whose control arm is a
// passthrough variation.
const trackingFeaturesJSON = `{
  "ramped": {
    "defaultValue": "default",
    "rules": [
      {
        "key": "ramp",
        "coverage": 1,
        "variations": ["treatment", "default"],
        "weights": [0, 1],
        "meta": [{"key": "0"}, {"key": "1", "passthrough": true}]
      },
      {"force": "fallthrough-value"}
    ]
  },
  "ramped-treatment": {
    "defaultValue": "default",
    "rules": [
      {
        "key": "ramp-t",
        "coverage": 1,
        "variations": ["treatment", "default"],
        "weights": [1, 0],
        "meta": [{"key": "0"}, {"key": "1", "passthrough": true}]
      },
      {"force": "fallthrough-value"}
    ]
  },
  "ramped-into-experiment": {
    "defaultValue": "default",
    "rules": [
      {
        "key": "ramp2",
        "coverage": 1,
        "variations": ["treatment", "default"],
        "weights": [0, 1],
        "meta": [{"key": "0"}, {"key": "1", "passthrough": true}]
      },
      {"key": "exp2", "coverage": 1, "variations": ["a", "b"], "weights": [1, 0]}
    ]
  },
  "parent": {
    "defaultValue": "off",
    "rules": [
      {"key": "parent-exp", "coverage": 1, "variations": ["on", "off"], "weights": [1, 0]}
    ]
  },
  "child": {
    "defaultValue": "child-default",
    "rules": [
      {"parentConditions": [{"id": "parent", "condition": {"value": "on"}}], "force": "child-on"}
    ]
  },
  "child-twice": {
    "defaultValue": "child-default",
    "rules": [
      {"parentConditions": [{"id": "parent", "condition": {"value": "nope"}}], "force": "r1"},
      {"parentConditions": [{"id": "parent", "condition": {"value": "on"}}], "force": "r2"}
    ]
  },
  "cycle-a": {
    "defaultValue": "a-default",
    "rules": [{"parentConditions": [{"id": "cycle-b", "condition": {"value": "x"}}], "force": "a-forced"}]
  },
  "cycle-b": {
    "defaultValue": "b-default",
    "rules": [{"parentConditions": [{"id": "cycle-a", "condition": {"value": "x"}}], "force": "b-forced"}]
  }
}`

type trackedExposure struct {
	key         string
	variationId int
	passthrough bool
	featureId   string
}

type trackedUsage struct {
	key    string
	source FeatureResultSource
}

// trackingRecorder doubles as a Plugin and as the body of the two client
// callbacks.
type trackingRecorder struct {
	exposures []trackedExposure
	usage     []trackedUsage
}

func (r *trackingRecorder) Init(*Client) error { return nil }
func (r *trackingRecorder) Close() error       { return nil }
func (r *trackingRecorder) OnExperimentViewed(_ context.Context, exp *Experiment, res *ExperimentResult) {
	r.exposures = append(r.exposures, trackedExposure{exp.Key, res.VariationId, res.Passthrough, res.FeatureId})
}
func (r *trackingRecorder) OnFeatureEvaluated(_ context.Context, key string, res *FeatureResult) {
	r.usage = append(r.usage, trackedUsage{key, res.Source})
}

func exposures(t *testing.T, data []TrackingData) []trackedExposure {
	t.Helper()
	out := make([]trackedExposure, 0, len(data))
	for _, d := range data {
		out = append(out, trackedExposure{d.Experiment.Key, d.Result.VariationId, d.Result.Passthrough, d.Result.FeatureId})
	}
	return out
}

func newTrackingTestClient(t *testing.T) (*Client, *trackingRecorder, *trackingRecorder) {
	t.Helper()
	plugin := &trackingRecorder{}
	callbacks := &trackingRecorder{}
	client, err := NewClient(context.Background(),
		WithJsonFeatures(trackingFeaturesJSON),
		WithAttributes(Attributes{"id": "user-1"}),
		WithDeferredTracking(),
		WithExperimentCallback(func(ctx context.Context, exp *Experiment, res *ExperimentResult, _ any) {
			callbacks.OnExperimentViewed(ctx, exp, res)
		}),
		WithFeatureUsageCallback(func(ctx context.Context, key string, res *FeatureResult, _ any) {
			callbacks.OnFeatureEvaluated(ctx, key, res)
		}),
		WithPlugins(plugin),
	)
	require.NoError(t, err)
	return client, callbacks, plugin
}

func TestPassthroughAssignmentTracked(t *testing.T) {
	ctx := context.Background()

	t.Run("control arm falls through to a force rule and is tracked", func(t *testing.T) {
		client, callbacks, plugin := newTrackingTestClient(t)
		res := client.EvalFeature(ctx, "ramped")

		require.Equal(t, "fallthrough-value", res.Value)
		require.Equal(t, ForceResultSource, res.Source)
		require.False(t, res.InExperiment())

		want := []trackedExposure{{"ramp", 1, true, "ramped"}}
		require.Equal(t, want, callbacks.exposures)
		require.Equal(t, want, plugin.exposures)
		require.Equal(t, want, exposures(t, client.DeferredTrackingCalls()))
		require.Equal(t, []trackedUsage{{"ramped", ForceResultSource}}, callbacks.usage)
	})

	t.Run("control arm falls through to an experiment rule: both tracked, in evaluation order", func(t *testing.T) {
		client, callbacks, plugin := newTrackingTestClient(t)
		res := client.EvalFeature(ctx, "ramped-into-experiment")

		require.Equal(t, "a", res.Value)
		require.True(t, res.InExperiment())

		want := []trackedExposure{
			{"ramp2", 1, true, "ramped-into-experiment"},
			{"exp2", 0, false, "ramped-into-experiment"},
		}
		require.Equal(t, want, callbacks.exposures)
		require.Equal(t, want, plugin.exposures)
		require.Equal(t, want, exposures(t, client.DeferredTrackingCalls()))
	})

	t.Run("treatment arm serves the rule value with a single exposure", func(t *testing.T) {
		client, callbacks, plugin := newTrackingTestClient(t)
		res := client.EvalFeature(ctx, "ramped-treatment")

		require.Equal(t, "treatment", res.Value)
		require.True(t, res.InExperiment())

		want := []trackedExposure{{"ramp-t", 0, false, "ramped-treatment"}}
		require.Equal(t, want, callbacks.exposures)
		require.Equal(t, want, plugin.exposures)
		require.Equal(t, want, exposures(t, client.DeferredTrackingCalls()))
	})
}

func TestPrerequisiteTracking(t *testing.T) {
	ctx := context.Background()

	t.Run("a prerequisite's experiment assignment is tracked", func(t *testing.T) {
		client, callbacks, plugin := newTrackingTestClient(t)
		res := client.EvalFeature(ctx, "child")

		require.Equal(t, "child-on", res.Value)

		wantExposures := []trackedExposure{{"parent-exp", 0, false, "parent"}}
		require.Equal(t, wantExposures, callbacks.exposures)
		require.Equal(t, wantExposures, plugin.exposures)
		require.Equal(t, wantExposures, exposures(t, client.DeferredTrackingCalls()))

		wantUsage := []trackedUsage{
			{"parent", ExperimentResultSource},
			{"child", ForceResultSource},
		}
		require.Equal(t, wantUsage, callbacks.usage)
		require.Equal(t, wantUsage, plugin.usage)
	})

	t.Run("a prerequisite consulted by several rules is reported once", func(t *testing.T) {
		client, callbacks, _ := newTrackingTestClient(t)
		res := client.EvalFeature(ctx, "child-twice")

		require.Equal(t, "r2", res.Value)
		require.Equal(t, []trackedExposure{{"parent-exp", 0, false, "parent"}}, callbacks.exposures)
		require.Equal(t, []trackedUsage{
			{"parent", ExperimentResultSource},
			{"child-twice", ForceResultSource},
		}, callbacks.usage)
	})

	t.Run("callbacks fire per call; the deferred buffer dedupes across calls", func(t *testing.T) {
		client, callbacks, _ := newTrackingTestClient(t)
		client.EvalFeature(ctx, "child")
		client.EvalFeature(ctx, "child")

		require.Len(t, callbacks.exposures, 2)
		require.Len(t, callbacks.usage, 4)
		require.Len(t, client.DeferredTrackingCalls(), 1)
	})
}

func TestDeferredTracking(t *testing.T) {
	ctx := context.Background()

	t.Run("works without callbacks or plugins", func(t *testing.T) {
		client, err := NewClient(ctx,
			WithJsonFeatures(trackingFeaturesJSON),
			WithAttributes(Attributes{"id": "user-1"}),
			WithDeferredTracking(),
		)
		require.NoError(t, err)

		client.EvalFeature(ctx, "ramped-into-experiment")
		require.Equal(t, []trackedExposure{
			{"ramp2", 1, true, "ramped-into-experiment"},
			{"exp2", 0, false, "ramped-into-experiment"},
		}, exposures(t, client.DeferredTrackingCalls()))
	})

	t.Run("accumulates across evaluations and clears", func(t *testing.T) {
		client, _, _ := newTrackingTestClient(t)
		client.EvalFeature(ctx, "ramped")
		client.EvalFeature(ctx, "child")

		require.Equal(t, []trackedExposure{
			{"ramp", 1, true, "ramped"},
			{"parent-exp", 0, false, "parent"},
		}, exposures(t, client.DeferredTrackingCalls()))

		client.ClearDeferredTrackingCalls()
		require.Empty(t, client.DeferredTrackingCalls())

		client.EvalFeature(ctx, "ramped")
		require.Len(t, client.DeferredTrackingCalls(), 1)
	})

	t.Run("nil without WithDeferredTracking", func(t *testing.T) {
		client, err := NewClient(ctx,
			WithJsonFeatures(trackingFeaturesJSON),
			WithAttributes(Attributes{"id": "user-1"}),
		)
		require.NoError(t, err)
		client.EvalFeature(ctx, "ramped")
		require.Nil(t, client.DeferredTrackingCalls())
		client.ClearDeferredTrackingCalls()
	})

	t.Run("children armed separately have isolated buffers", func(t *testing.T) {
		base, err := NewClient(ctx,
			WithJsonFeatures(trackingFeaturesJSON),
			WithAttributes(Attributes{"id": "user-1"}),
		)
		require.NoError(t, err)

		child1, err := base.WithDeferredTracking()
		require.NoError(t, err)
		child2, err := base.WithDeferredTracking()
		require.NoError(t, err)

		child1.EvalFeature(ctx, "ramped")
		require.Len(t, child1.DeferredTrackingCalls(), 1)
		require.Empty(t, child2.DeferredTrackingCalls())
		require.Nil(t, base.DeferredTrackingCalls())
	})

	t.Run("clones of an armed client share its buffer", func(t *testing.T) {
		client, _, _ := newTrackingTestClient(t)
		child, err := client.WithExtraData("request-scoped")
		require.NoError(t, err)

		child.EvalFeature(ctx, "ramped")
		require.Len(t, client.DeferredTrackingCalls(), 1)
	})

	t.Run("re-arming a clone detaches it from the parent's buffer", func(t *testing.T) {
		parent, _, _ := newTrackingTestClient(t)
		child, err := parent.WithDeferredTracking()
		require.NoError(t, err)

		child.EvalFeature(ctx, "ramped")
		require.Len(t, child.DeferredTrackingCalls(), 1)
		require.Empty(t, parent.DeferredTrackingCalls())
	})

	t.Run("a shared armed client accumulates all users without cross-user dedupe", func(t *testing.T) {
		shared, _, _ := newTrackingTestClient(t)
		user1, err := shared.WithAttributes(Attributes{"id": "user-1"})
		require.NoError(t, err)
		user2, err := shared.WithAttributes(Attributes{"id": "user-2"})
		require.NoError(t, err)

		user1.EvalFeature(ctx, "ramped-treatment")
		user2.EvalFeature(ctx, "ramped-treatment")

		calls := shared.DeferredTrackingCalls()
		require.Len(t, calls, 2)
		require.NotEqual(t, calls[0].Result.HashValue, calls[1].Result.HashValue)
	})
}

func TestRunExperimentTracking(t *testing.T) {
	ctx := context.Background()

	t.Run("direct experiment and its prerequisite assignments are both reported", func(t *testing.T) {
		client, callbacks, _ := newTrackingTestClient(t)
		var exp Experiment
		require.NoError(t, json.Unmarshal([]byte(`{
			"key": "direct",
			"variations": ["x", "y"],
			"weights": [1, 0],
			"coverage": 1,
			"parentConditions": [{"id": "parent", "condition": {"value": "on"}}]
		}`), &exp))

		res := client.RunExperiment(ctx, &exp)
		require.True(t, res.InExperiment)
		require.Equal(t, "x", res.Value)

		want := []trackedExposure{
			{"parent-exp", 0, false, "parent"},
			{"direct", 0, false, ""},
		}
		require.Equal(t, want, callbacks.exposures)
		require.Equal(t, want, exposures(t, client.DeferredTrackingCalls()))
		require.Equal(t, []trackedUsage{{"parent", ExperimentResultSource}}, callbacks.usage)
	})

	t.Run("forced variations are served but not tracked (JS parity)", func(t *testing.T) {
		client, callbacks, plugin := newTrackingTestClient(t)
		force := 1
		exp := Experiment{Key: "forced", Variations: []FeatureValue{"x", "y"}, Force: &force}

		res := client.RunExperiment(ctx, &exp)
		require.True(t, res.InExperiment)
		require.False(t, res.HashUsed)
		require.Equal(t, "y", res.Value)

		require.Empty(t, callbacks.exposures)
		require.Empty(t, plugin.exposures)
		require.Empty(t, client.DeferredTrackingCalls())
	})
}

func TestFeatureUsageEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown feature usage is reported", func(t *testing.T) {
		client, callbacks, _ := newTrackingTestClient(t)
		res := client.EvalFeature(ctx, "no-such-feature")

		require.Equal(t, UnknownFeatureResultSource, res.Source)
		require.Equal(t, []trackedUsage{{"no-such-feature", UnknownFeatureResultSource}}, callbacks.usage)
		require.Empty(t, client.DeferredTrackingCalls())
	})

	t.Run("cyclic prerequisites report each feature once", func(t *testing.T) {
		client, callbacks, _ := newTrackingTestClient(t)
		res := client.EvalFeature(ctx, "cycle-a")

		require.Equal(t, CyclicPrerequisiteResultSource, res.Source)
		require.Equal(t, []trackedUsage{
			{"cycle-a", CyclicPrerequisiteResultSource},
			{"cycle-b", CyclicPrerequisiteResultSource},
		}, callbacks.usage)
	})
}

func TestTrackingDataShape(t *testing.T) {
	td := TrackingData{
		Experiment: &Experiment{Key: "exp"},
		Result:     &ExperimentResult{HashAttribute: "id", HashValue: "u1", VariationId: 2},
	}

	t.Run("DedupeKey matches the JS SDK's getExperimentDedupeKey", func(t *testing.T) {
		require.Equal(t, "idu1exp2", td.DedupeKey())
	})

	t.Run("serializes with the JS TrackingData field names", func(t *testing.T) {
		b, err := json.Marshal(td)
		require.NoError(t, err)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &m))
		require.Contains(t, m, "experiment")
		require.Contains(t, m, "result")
	})
}

func TestConcurrentDeferredTracking(t *testing.T) {
	ctx := context.Background()
	client, err := NewClient(ctx,
		WithJsonFeatures(trackingFeaturesJSON),
		WithAttributes(Attributes{"id": "user-1"}),
		WithDeferredTracking(),
	)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.EvalFeature(ctx, "child")
			client.EvalFeature(ctx, "ramped")
		}()
	}
	wg.Wait()

	require.ElementsMatch(t, []trackedExposure{
		{"parent-exp", 0, false, "parent"},
		{"ramp", 1, true, "ramped"},
	}, exposures(t, client.DeferredTrackingCalls()))
}
