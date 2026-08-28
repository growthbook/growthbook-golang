package growthbook

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// A passthrough variation does not serve its value — evaluation falls
// through to the next rule — but the assignment is still an exposure and
// must reach ExperimentCallback and plugins, as in the JS and Python SDKs.
// The canonical producer is a monitored ramp step: treatment serves the
// rule value, control is a passthrough arm, and the ramp's health check
// compares exposure counts between the two.
const passthroughFeaturesJSON = `{
  "flag": {
    "defaultValue": "default",
    "rules": [
      {
        "key": "ramp_1",
        "coverage": 1,
        "hashAttribute": "id",
        "variations": ["treatment", "default"],
        "weights": [0.5, 0.5],
        "meta": [{"key": "0"}, {"key": "1", "passthrough": true}]
      },
      {"force": "lower-rule"}
    ]
  },
  "child": {
    "defaultValue": "child-default",
    "rules": [
      {
        "parentConditions": [{"id": "flag", "condition": {"value": {"$exists": true}}}],
        "force": "child-forced"
      }
    ]
  }
}`

type viewed struct {
	key         string
	variationID int
	passthrough bool
	featureID   string
}

type recordingPlugin struct{ viewed []viewed }

func (p *recordingPlugin) Init(*Client) error { return nil }
func (p *recordingPlugin) Close() error       { return nil }
func (p *recordingPlugin) OnFeatureEvaluated(context.Context, string, *FeatureResult) {
}
func (p *recordingPlugin) OnExperimentViewed(_ context.Context, exp *Experiment, res *ExperimentResult) {
	p.viewed = append(p.viewed, viewed{exp.Key, res.VariationId, res.Passthrough, res.FeatureId})
}

func passthroughClient(t *testing.T, id string) (*Client, *[]viewed, *recordingPlugin) {
	t.Helper()
	var fromCallback []viewed
	plugin := &recordingPlugin{}
	client, err := NewClient(context.Background(),
		WithJsonFeatures(passthroughFeaturesJSON),
		WithAttributes(Attributes{"id": id}),
		WithExperimentCallback(func(_ context.Context, exp *Experiment, res *ExperimentResult, _ any) {
			fromCallback = append(fromCallback, viewed{exp.Key, res.VariationId, res.Passthrough, res.FeatureId})
		}),
		WithPlugins(plugin),
	)
	require.NoError(t, err)
	return client, &fromCallback, plugin
}

// idsByArm finds one user id per arm of the ramp_1 experiment.
func idsByArm(t *testing.T) (treatmentID, controlID string) {
	t.Helper()
	for i := 0; treatmentID == "" || controlID == ""; i++ {
		require.Less(t, i, 1000, "could not find ids for both arms")
		id := fmt.Sprintf("user-%d", i)
		client, _, _ := passthroughClient(t, id)
		rule := client.Features()["flag"].Rules[0]
		res := client.RunExperiment(context.Background(), experimentFromFeatureRule("flag", &rule))
		if !res.InExperiment {
			continue
		}
		if res.VariationId == 0 && treatmentID == "" {
			treatmentID = id
		}
		if res.VariationId == 1 && controlID == "" {
			controlID = id
		}
	}
	return treatmentID, controlID
}

func TestPassthroughAssignmentIsTracked(t *testing.T) {
	ctx := context.Background()
	treatmentID, controlID := idsByArm(t)

	t.Run("control arm falls through but is tracked", func(t *testing.T) {
		client, fromCallback, plugin := passthroughClient(t, controlID)
		res := client.EvalFeature(ctx, "flag")

		require.Equal(t, "lower-rule", res.Value)
		require.Equal(t, ForceResultSource, res.Source)
		require.False(t, res.InExperiment())

		want := []viewed{{key: "ramp_1", variationID: 1, passthrough: true, featureID: "flag"}}
		require.Equal(t, want, *fromCallback)
		require.Equal(t, want, plugin.viewed)
	})

	t.Run("treatment arm is unchanged", func(t *testing.T) {
		client, fromCallback, plugin := passthroughClient(t, treatmentID)
		res := client.EvalFeature(ctx, "flag")

		require.Equal(t, "treatment", res.Value)
		require.True(t, res.InExperiment())

		want := []viewed{{key: "ramp_1", variationID: 0, passthrough: false, featureID: "flag"}}
		require.Equal(t, want, *fromCallback)
		require.Equal(t, want, plugin.viewed)
	})

	t.Run("prerequisite's passthrough assignment is not attributed to the dependent feature", func(t *testing.T) {
		client, fromCallback, plugin := passthroughClient(t, controlID)
		res := client.EvalFeature(ctx, "child")

		require.Equal(t, "child-forced", res.Value)
		require.Empty(t, *fromCallback)
		require.Empty(t, plugin.viewed)
	})

	t.Run("evaluator state does not leak between evaluations", func(t *testing.T) {
		client, fromCallback, _ := passthroughClient(t, controlID)
		client.EvalFeature(ctx, "flag")
		client.EvalFeature(ctx, "child")
		client.EvalFeature(ctx, "flag")
		require.Len(t, *fromCallback, 2)
	})
}
