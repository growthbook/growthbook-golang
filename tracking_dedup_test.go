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

func TestExperimentTrackingReemitsAfterFeatureChange(t *testing.T) {
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

	// Changing features clears the dedup cache, so tracking re-emits.
	require.NoError(t, client.SetJSONFeatures(expFeatureJSON))
	client.EvalFeature(ctx, "feat")
	require.Equal(t, 2, calls)
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
