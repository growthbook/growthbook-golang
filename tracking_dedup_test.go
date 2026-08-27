package growthbook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

const expFeatureJSON = `{"feat":{"defaultValue":0,"rules":[{"key":"my-exp","variations":[0,1],"hashAttribute":"id"}]}}`

func TestExperimentTrackingDeduplicated(t *testing.T) {
	var calls int
	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "user-1"}),
		WithJsonFeatures(expFeatureJSON),
		WithExperimentCallback(func(_ context.Context, _ *Experiment, _ *ExperimentResult, _ any) {
			calls++
		}),
	)
	require.NoError(t, err)

	res := client.EvalFeature(ctx, "feat")
	require.True(t, res.InExperiment(), "user should be in the experiment")

	for i := 0; i < 5; i++ {
		client.EvalFeature(ctx, "feat")
	}
	require.Equal(t, 1, calls, "experiment_viewed should fire once for a repeated assignment")
}

func TestExperimentTrackingPerUser(t *testing.T) {
	var calls int
	parent, err := NewClient(ctx,
		WithJsonFeatures(expFeatureJSON),
		WithExperimentCallback(func(_ context.Context, _ *Experiment, _ *ExperimentResult, _ any) {
			calls++
		}),
	)
	require.NoError(t, err)

	c1, err := parent.WithAttributes(Attributes{"id": "user-1"})
	require.NoError(t, err)
	c2, err := parent.WithAttributes(Attributes{"id": "user-2"})
	require.NoError(t, err)

	c1.EvalFeature(ctx, "feat")
	c1.EvalFeature(ctx, "feat")
	c2.EvalFeature(ctx, "feat")
	c2.EvalFeature(ctx, "feat")

	require.Equal(t, 2, calls, "each distinct user should be tracked once")
}

func TestExperimentTrackingPersistsAcrossFeatureUpdates(t *testing.T) {
	var calls int
	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "user-1"}),
		WithJsonFeatures(expFeatureJSON),
		WithExperimentCallback(func(_ context.Context, _ *Experiment, _ *ExperimentResult, _ any) {
			calls++
		}),
	)
	require.NoError(t, err)

	client.EvalFeature(ctx, "feat")
	client.EvalFeature(ctx, "feat")
	require.Equal(t, 1, calls)

	// A feature update must NOT reset the dedup cache — otherwise every poll
	// that returns 200 would re-emit tracking for unchanged assignments.
	require.NoError(t, client.SetJSONFeatures(expFeatureJSON))
	client.EvalFeature(ctx, "feat")
	require.Equal(t, 1, calls)
}

func TestFeatureUsageNotDeduplicated(t *testing.T) {
	var usage int
	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "user-1"}),
		WithJsonFeatures(expFeatureJSON),
		WithFeatureUsageCallback(func(_ context.Context, _ string, _ *FeatureResult, _ any) {
			usage++
		}),
	)
	require.NoError(t, err)

	client.EvalFeature(ctx, "feat")
	client.EvalFeature(ctx, "feat")
	client.EvalFeature(ctx, "feat")
	require.Equal(t, 3, usage, "feature_evaluated must fire on every evaluation")
}

func TestExperimentTrackingRequiresConsumer(t *testing.T) {
	exp := &Experiment{Key: "e"}
	res := &ExperimentResult{HashAttribute: "id", HashValue: "1", VariationId: 0}

	// With no callback or plugin, nothing consumes tracking, so the dedup set is
	// never populated.
	noConsumer, err := NewClient(ctx)
	require.NoError(t, err)
	require.False(t, noConsumer.shouldTrackExperiment(exp, res))

	// With a consumer, the first exposure tracks and repeats are deduplicated.
	withCb, err := NewClient(ctx, WithExperimentCallback(func(context.Context, *Experiment, *ExperimentResult, any) {}))
	require.NoError(t, err)
	require.True(t, withCb.shouldTrackExperiment(exp, res))
	require.False(t, withCb.shouldTrackExperiment(exp, res))
}

func TestTrackedSetBoundedLRU(t *testing.T) {
	s := &trackedSet{max: 2}
	k := func(v int) trackKey { return trackKey{experimentKey: "e", variationID: v} }

	require.True(t, s.markOnce(k(1)))
	require.True(t, s.markOnce(k(2)))
	require.True(t, s.markOnce(k(3))) // over capacity -> evicts the LRU entry k(1)

	require.False(t, s.markOnce(k(3))) // still present
	require.False(t, s.markOnce(k(2))) // still present
	require.True(t, s.markOnce(k(1)))  // was evicted -> treated as new
}

func TestTrackKeyNoCollision(t *testing.T) {
	s := &trackedSet{max: 100}
	// A delimiter-joined key ("a::b") would conflate these two distinct
	// assignments; a struct key keeps them separate.
	require.True(t, s.markOnce(trackKey{hashValue: "a", experimentKey: "b"}))
	require.True(t, s.markOnce(trackKey{hashValue: "a::b", experimentKey: ""}))
}
