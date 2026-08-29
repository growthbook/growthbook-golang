package growthbook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The bandit rules carry variations only under contextualVariations, pinning
// that bandit-aware SDKs evaluate them while older SDKs skip them. Leaf
// weights of [1,0]/[0,1] make assignments deterministic.
const banditFeaturesJSON = `{
  "bandit-flag": {
    "defaultValue": "default",
    "rules": [
      {
        "key": "bandit-exp",
        "coverage": 1,
        "contextualBanditRef": "cb-1",
        "contextualVariations": ["a", "b"],
        "weights": [0.5, 0.5],
        "meta": [{"key": "0"}, {"key": "1"}]
      }
    ]
  },
  "bandit-no-match": {
    "defaultValue": "default",
    "rules": [
      {
        "key": "bandit-nm",
        "coverage": 1,
        "contextualBanditRef": "cb-us-only",
        "contextualVariations": ["a", "b"],
        "weights": [0, 1]
      }
    ]
  },
  "bandit-missing-ref": {
    "defaultValue": "default",
    "rules": [
      {
        "key": "bandit-mr",
        "coverage": 1,
        "contextualBanditRef": "cb-nope",
        "contextualVariations": ["a", "b"],
        "weights": [0, 1]
      }
    ]
  },
  "bandit-no-weights": {
    "defaultValue": "default",
    "rules": [
      {
        "key": "bandit-nw",
        "coverage": 1,
        "contextualBanditRef": "cb-us-only",
        "contextualVariations": ["a", "b"]
      }
    ]
  }
}`

const banditDefsJSON = `{
  "cb-1": {
    "banditVersion": 3,
    "contexts": [
      {"leafId": 10, "condition": {"country": "us"}, "weights": [1, 0]},
      {"leafId": 20, "condition": {"country": "nz"}, "weights": [0, 1]},
      {"leafId": 30, "condition": {}, "weights": [1, 0]}
    ]
  },
  "cb-us-only": {
    "contexts": [
      {"leafId": 10, "condition": {"country": "us"}, "weights": [1, 0]}
    ]
  }
}`

func mustBanditDefs(t *testing.T, raw string) ContextualBanditDefinitions {
	t.Helper()
	var defs ContextualBanditDefinitions
	require.NoError(t, json.Unmarshal([]byte(raw), &defs))
	return defs
}

func newBanditTestClient(t *testing.T, attrs Attributes, extra ...ClientOption) *Client {
	t.Helper()
	opts := []ClientOption{
		WithJsonFeatures(banditFeaturesJSON),
		WithContextualBandits(mustBanditDefs(t, banditDefsJSON)),
		WithAttributes(attrs),
	}
	client, err := NewClient(context.Background(), append(opts, extra...)...)
	require.NoError(t, err)
	return client
}

func TestContextualBanditLeafSelection(t *testing.T) {
	ctx := context.Background()
	three := 3

	t.Run("first leaf whose condition passes supplies the weights", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"})
		res := client.EvalFeature(ctx, "bandit-flag")

		require.Equal(t, "a", res.Value)
		require.True(t, res.InExperiment())
		r := res.ExperimentResult
		require.Equal(t, 10, *r.LeafId)
		require.Equal(t, []float64{1, 0}, r.VariationWeights)
		require.Equal(t, three, *r.BanditVersion)
	})

	t.Run("a later leaf matches a different user", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "nz"})
		res := client.EvalFeature(ctx, "bandit-flag")

		require.Equal(t, "b", res.Value)
		require.Equal(t, 20, *res.ExperimentResult.LeafId)
		require.Equal(t, []float64{0, 1}, res.ExperimentResult.VariationWeights)
	})

	t.Run("an empty condition acts as a catch-all leaf", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "de"})
		res := client.EvalFeature(ctx, "bandit-flag")

		require.Equal(t, "a", res.Value)
		require.Equal(t, 30, *res.ExperimentResult.LeafId)
	})

	t.Run("no matching leaf assigns with aggregate weights under the fallback leaf", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "de"})
		res := client.EvalFeature(ctx, "bandit-no-match")

		require.Equal(t, "b", res.Value)
		r := res.ExperimentResult
		require.Equal(t, -1, *r.LeafId)
		require.Equal(t, []float64{0, 1}, r.VariationWeights)
		require.Nil(t, r.BanditVersion)
	})

	t.Run("a missing definition assigns with aggregate weights and no bandit fields", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"})
		res := client.EvalFeature(ctx, "bandit-missing-ref")

		require.Equal(t, "b", res.Value)
		require.True(t, res.InExperiment())
		require.Nil(t, res.ExperimentResult.LeafId)
		require.Nil(t, res.ExperimentResult.VariationWeights)
		require.Nil(t, res.Experiment.ContextualBandit)
	})

	t.Run("no matching leaf and no rule weights falls back to equal weights", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "de"})
		res := client.EvalFeature(ctx, "bandit-no-weights")

		require.True(t, res.InExperiment())
		require.Equal(t, -1, *res.ExperimentResult.LeafId)
		require.Equal(t, []float64{0.5, 0.5}, res.ExperimentResult.VariationWeights)
	})

	t.Run("client-forced variations carry no bandit fields", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
			WithForcedVariations(ForcedVariationsMap{"bandit-exp": 1}))
		res := client.EvalFeature(ctx, "bandit-flag")

		require.Equal(t, "b", res.Value)
		require.False(t, res.ExperimentResult.HashUsed)
		require.Nil(t, res.ExperimentResult.LeafId)
		require.Nil(t, res.Experiment.ContextualBandit)
	})
}

func TestContextualBanditTracking(t *testing.T) {
	ctx := context.Background()
	client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"}, WithDeferredTracking())

	client.EvalFeature(ctx, "bandit-flag")
	calls := client.DeferredTrackingCalls()
	require.Len(t, calls, 1)
	require.Equal(t, 10, *calls[0].Result.LeafId)
	require.Equal(t, 10, calls[0].Experiment.ContextualBandit.LeafId)

	b, err := json.Marshal(calls[0])
	require.NoError(t, err)
	var m map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "10", string(m["result"]["leafId"]))
	require.Equal(t, "[1,0]", string(m["result"]["variationWeights"]))
	require.Equal(t, "3", string(m["result"]["banditVersion"]))

	client.ClearDeferredTrackingCalls()
	client.EvalFeature(ctx, "bandit-missing-ref")
	calls = client.DeferredTrackingCalls()
	require.Len(t, calls, 1)
	nb, err := json.Marshal(calls[0])
	require.NoError(t, err)
	require.NotContains(t, string(nb), "leafId")
}

func TestContextualBanditRobustness(t *testing.T) {
	ctx := context.Background()

	t.Run("empty contextualVariations falls through without panicking", func(t *testing.T) {
		client, err := NewClient(ctx,
			WithJsonFeatures(`{"f": {"defaultValue": "d", "rules": [
				{"contextualBanditRef": "x", "contextualVariations": []},
				{"force": "next-rule"}
			]}}`),
			WithAttributes(Attributes{"id": "u"}))
		require.NoError(t, err)
		res := client.EvalFeature(ctx, "f")
		require.Equal(t, "next-rule", res.Value)
	})

	t.Run("RunExperiment with no variations does not panic", func(t *testing.T) {
		client, err := NewClient(ctx, WithAttributes(Attributes{"id": "u"}))
		require.NoError(t, err)
		res := client.RunExperiment(ctx, &Experiment{Key: "empty"})
		require.False(t, res.InExperiment)
		require.Nil(t, res.Value)
	})

	t.Run("leaf weights of the wrong length are sanitized and reported as used", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
			WithContextualBandits(mustBanditDefs(t, `{
				"cb-1": {"contexts": [{"leafId": 5, "condition": {}, "weights": [1, 0, 0]}]}
			}`)))
		res := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, res.InExperiment())
		require.Equal(t, 5, *res.ExperimentResult.LeafId)
		require.Equal(t, []float64{0.5, 0.5}, res.ExperimentResult.VariationWeights)
	})

	t.Run("a malformed context is dropped without discarding its siblings", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "nz"},
			WithContextualBandits(mustBanditDefs(t, `{
				"cb-1": {"contexts": [
					{"leafId": "junk", "condition": {}, "weights": [1, 0]},
					{"leafId": 20, "condition": {"country": "nz"}, "weights": [0, 1]}
				]}
			}`)))
		res := client.EvalFeature(ctx, "bandit-flag")
		require.Equal(t, "b", res.Value)
		require.Equal(t, 20, *res.ExperimentResult.LeafId)
	})

	t.Run("sticky-bucketed assignments carry no bandit fields", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
			WithStickyBucketService(NewInMemoryStickyBucketService()))

		first := client.EvalFeature(ctx, "bandit-flag")
		require.False(t, first.ExperimentResult.StickyBucketUsed)
		require.Equal(t, 10, *first.ExperimentResult.LeafId)

		second := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, second.ExperimentResult.StickyBucketUsed)
		require.Nil(t, second.ExperimentResult.LeafId)
		require.Nil(t, second.ExperimentResult.VariationWeights)
	})

	t.Run("undecodable encrypted bandits do not block the feature update", func(t *testing.T) {
		client, err := NewClient(ctx,
			WithAttributes(Attributes{"id": "u"}),
			WithDecryptionKey("Ns04T5n9+59rl2x3SlNHtQ=="))
		require.NoError(t, err)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": {"f": {"defaultValue": 1}},
			"encryptedContextualBandits": "not-a-valid-blob",
			"dateUpdated": "2030-01-01T00:00:00Z"
		}`))
		res := client.EvalFeature(ctx, "f")
		require.Equal(t, 1.0, res.Value)
	})
}

func TestFeatureRuleMarshalRoundTrip(t *testing.T) {
	ctx := context.Background()
	client, err := NewClient(ctx,
		WithJsonFeatures(`{
			"exp": {"defaultValue": "d", "rules": [{"key": "e", "coverage": 1, "variations": ["a", "b"], "weights": [1, 0]}]},
			"null-force": {"defaultValue": "d", "rules": [{"force": null}]}
		}`),
		WithAttributes(Attributes{"id": "u"}))
	require.NoError(t, err)

	b, err := json.Marshal(client.Features())
	require.NoError(t, err)
	reloaded, err := NewClient(ctx, WithAttributes(Attributes{"id": "u"}))
	require.NoError(t, err)
	require.NoError(t, reloaded.SetJSONFeatures(string(b)))

	res := reloaded.EvalFeature(ctx, "exp")
	require.Equal(t, "a", res.Value)
	require.Equal(t, ExperimentResultSource, res.Source)

	nf := reloaded.EvalFeature(ctx, "null-force")
	require.Nil(t, nf.Value)
	require.Equal(t, ForceResultSource, nf.Source)
}

func TestSetContextualBandits(t *testing.T) {
	ctx := context.Background()
	client, err := NewClient(ctx,
		WithJsonFeatures(banditFeaturesJSON),
		WithAttributes(Attributes{"id": "u1", "country": "nz"}),
	)
	require.NoError(t, err)

	res := client.EvalFeature(ctx, "bandit-flag")
	require.Nil(t, res.ExperimentResult.LeafId, "no definitions set yet")

	require.NoError(t, client.SetContextualBandits(mustBanditDefs(t, banditDefsJSON)))

	res = client.EvalFeature(ctx, "bandit-flag")
	require.Equal(t, 20, *res.ExperimentResult.LeafId)
}

func TestFeatureRuleUnmarshalErrors(t *testing.T) {
	var rule FeatureRule
	require.Error(t, json.Unmarshal([]byte(`{"force": }`), &rule))
	require.Error(t, json.Unmarshal([]byte(`"not-an-object"`), &rule))
}

func TestLooseContextualBanditPayloads(t *testing.T) {
	// A bandit blob the SDK cannot parse must not block the feature update
	// it arrived with; parseable definitions survive alongside broken ones.
	ctx := context.Background()
	client, err := NewClient(ctx, WithAttributes(Attributes{"id": "u1", "country": "nz"}))
	require.NoError(t, err)

	require.NoError(t, client.UpdateFromApiResponseJSON(`{
		"features": `+banditFeaturesJSON+`,
		"contextualBandits": {
			"cb-broken": {"contexts": [{"leafId": "not-a-number"}]},
			"cb-1": `+`{
				"contexts": [{"leafId": 20, "condition": {"country": "nz"}, "weights": [0, 1]}]
			}`+`
		},
		"dateUpdated": "2030-01-01T00:00:00Z"
	}`))

	res := client.EvalFeature(ctx, "bandit-flag")
	require.Equal(t, 20, *res.ExperimentResult.LeafId, "parseable definition survives")

	client2, err := NewClient(ctx, WithAttributes(Attributes{"id": "u1"}))
	require.NoError(t, err)
	require.NoError(t, client2.UpdateFromApiResponseJSON(`{
		"features": {"f": {"defaultValue": 1}},
		"contextualBandits": ["entirely-wrong-shape"],
		"dateUpdated": "2030-01-01T00:00:00Z"
	}`))
	res2 := client2.EvalFeature(ctx, "f")
	require.Equal(t, 1.0, res2.Value, "features still update")
}

func TestContextualBanditsFromApiResponse(t *testing.T) {
	ctx := context.Background()
	client, err := NewClient(ctx, WithAttributes(Attributes{"id": "u1", "country": "nz"}))
	require.NoError(t, err)

	require.NoError(t, client.UpdateFromApiResponseJSON(`{
		"features": `+banditFeaturesJSON+`,
		"contextualBandits": `+banditDefsJSON+`,
		"dateUpdated": "2030-01-01T00:00:00Z"
	}`))

	res := client.EvalFeature(ctx, "bandit-flag")
	require.Equal(t, "b", res.Value)
	require.Equal(t, 20, *res.ExperimentResult.LeafId)
}
