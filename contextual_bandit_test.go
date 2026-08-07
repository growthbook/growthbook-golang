package growthbook

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// banditClient builds a client from a full API payload (features + bandits).
func banditClient(t *testing.T, attrs Attributes, payload string) *Client {
	t.Helper()
	client, err := NewClient(ctx, WithAttributes(attrs))
	require.NoError(t, err)
	require.NoError(t, client.UpdateFromApiResponseJSON(payload))
	return client
}

const banditFeatureRule = `"bandit": {
  "defaultValue": "default",
  "rules": [{
    "key": "bandit-exp",
    "seed": "bandit-exp",
    "hashAttribute": "id",
    "hashVersion": 2,
    "coverage": 1,
    "contextualVariations": ["control", "treatment"],
    "weights": [0.5, 0.5],
    "meta": [{"key": "0"}, {"key": "1"}],
    "contextualBanditRef": "cb"
  }]
}`

func TestContextualBanditAppliesLeafWeights(t *testing.T) {
	payload := `{"features": {` + banditFeatureRule + `},
      "contextualBandits": {
        "cb": {"banditVersion": 7, "contexts": [{"leafId": 1, "condition": {}, "weights": [1, 0]}]}
      }, "dateUpdated": "2020-01-01T00:00:00Z"}`

	client := banditClient(t, Attributes{"id": "1"}, payload)
	res := client.EvalFeature(ctx, "bandit")

	// weights [1,0] send all traffic to variation 0.
	require.Equal(t, "control", res.Value)
	require.Equal(t, ExperimentResultSource, res.Source)
	require.NotNil(t, res.ExperimentResult.LeafId)
	require.Equal(t, 1, *res.ExperimentResult.LeafId)
	require.Equal(t, []float64{1, 0}, res.ExperimentResult.VariationWeights)
	require.NotNil(t, res.ExperimentResult.BanditVersion)
	require.Equal(t, 7, *res.ExperimentResult.BanditVersion)
}

func TestContextualBanditFirstMatchingLeafWins(t *testing.T) {
	payload := `{"features": {` + banditFeatureRule + `},
      "contextualBandits": {
        "cb": {"banditVersion": 3, "contexts": [
          {"leafId": 1, "condition": {"plan": "enterprise"}, "weights": [1, 0]},
          {"leafId": 2, "condition": {}, "weights": [0, 1]}
        ]}
      }, "dateUpdated": "2020-01-01T00:00:00Z"}`

	// Enterprise → leaf 1 (weights [1,0]) → control.
	ent := banditClient(t, Attributes{"id": "1", "plan": "enterprise"}, payload)
	res := ent.EvalFeature(ctx, "bandit")
	require.Equal(t, "control", res.Value)
	require.Equal(t, 1, *res.ExperimentResult.LeafId)

	// Free → falls through to the catch-all leaf 2 (weights [0,1]) → treatment.
	free := banditClient(t, Attributes{"id": "1", "plan": "free"}, payload)
	res = free.EvalFeature(ctx, "bandit")
	require.Equal(t, "treatment", res.Value)
	require.Equal(t, 2, *res.ExperimentResult.LeafId)
}

func TestContextualBanditFallbackWhenNoLeafMatches(t *testing.T) {
	payload := `{"features": {` + banditFeatureRule + `},
      "contextualBandits": {
        "cb": {"banditVersion": 5, "contexts": [
          {"leafId": 1, "condition": {"plan": "enterprise"}, "weights": [1, 0]}
        ]}
      }, "dateUpdated": "2020-01-01T00:00:00Z"}`

	client := banditClient(t, Attributes{"id": "1", "plan": "free"}, payload)
	res := client.EvalFeature(ctx, "bandit")

	// No leaf matched: definition present → leafId -1, fall back to the rule's weights.
	require.True(t, res.ExperimentResult.InExperiment)
	require.NotNil(t, res.ExperimentResult.LeafId)
	require.Equal(t, -1, *res.ExperimentResult.LeafId)
	require.Equal(t, []float64{0.5, 0.5}, res.ExperimentResult.VariationWeights)
	require.Equal(t, 5, *res.ExperimentResult.BanditVersion)
}

func TestContextualBanditRefMissingReportsNoMetadata(t *testing.T) {
	// Rule references a bandit that isn't in the payload → plain experiment, no metadata.
	payload := `{"features": {` + banditFeatureRule + `},
      "contextualBandits": {}, "dateUpdated": "2020-01-01T00:00:00Z"}`

	client := banditClient(t, Attributes{"id": "1"}, payload)
	res := client.EvalFeature(ctx, "bandit")

	require.True(t, res.ExperimentResult.InExperiment)
	require.Nil(t, res.ExperimentResult.LeafId)
	require.Nil(t, res.ExperimentResult.VariationWeights)
	require.Nil(t, res.ExperimentResult.BanditVersion)
}

func TestContextualBanditViaWithOption(t *testing.T) {
	// Bandits can be seeded directly via WithContextualBandits.
	leafID := 9
	version := 2
	features := `{"bandit": {"defaultValue": "default", "rules": [{
      "key": "e", "hashAttribute": "id", "coverage": 1,
      "contextualVariations": ["control", "treatment"], "weights": [0.5, 0.5],
      "meta": [{"key": "0"}, {"key": "1"}], "contextualBanditRef": "cb"}]}}`

	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "1"}),
		WithJsonFeatures(features),
		WithContextualBandits(ContextualBanditsMap{
			"cb": {BanditVersion: &version, Contexts: []ContextualBanditContext{
				{LeafId: &leafID, Weights: []float64{0, 1}},
			}},
		}),
	)
	require.NoError(t, err)

	res := client.EvalFeature(ctx, "bandit")
	require.Equal(t, "treatment", res.Value)
	require.Equal(t, 9, *res.ExperimentResult.LeafId)
}
