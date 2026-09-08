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

	t.Run("a matched leaf with invalid weights demotes to the fallback leaf", func(t *testing.T) {
		// Substituting repaired weights would report a leaf whose
		// propensities differ from the vector bucketing used; the Python SDK
		// demotes to leafId -1 and keeps the rule's aggregate weights.
		for name, weights := range map[string]string{
			"wrong length": `[1, 0, 0]`,
			"negative":     `[1.2, -0.2]`,
			"sum not ~1":   `[0.4, 0.1]`,
			"missing":      `null`,
		} {
			t.Run(name, func(t *testing.T) {
				client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
					WithContextualBandits(mustBanditDefs(t, `{
						"cb-1": {"contexts": [{"leafId": 5, "condition": {}, "weights": `+weights+`}]}
					}`)))
				res := client.EvalFeature(ctx, "bandit-flag")
				require.True(t, res.InExperiment())
				require.Equal(t, -1, *res.ExperimentResult.LeafId)
				require.Equal(t, []float64{0.5, 0.5}, res.ExperimentResult.VariationWeights,
					"reported weights must be the aggregate weights bucketing used")
			})
		}
	})

	t.Run("a matched leaf with a junk leafId demotes instead of routing to a sibling", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "nz"},
			WithContextualBandits(mustBanditDefs(t, `{
				"cb-1": {"contexts": [
					{"leafId": "junk", "condition": {}, "weights": [1, 0]},
					{"leafId": 20, "condition": {"country": "nz"}, "weights": [0, 1]}
				]}
			}`)))
		res := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, res.InExperiment())
		require.Equal(t, -1, *res.ExperimentResult.LeafId,
			"the catch-all leaf matched; its junk leafId demotes it rather than skipping to the sibling")
		require.Equal(t, []float64{0.5, 0.5}, res.ExperimentResult.VariationWeights)
	})

	t.Run("a type-malformed context aborts leaf selection", func(t *testing.T) {
		// Selection cannot know whether the junk context would have matched,
		// so a later valid leaf must not be used (Python SDK parity).
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "nz"},
			WithContextualBandits(mustBanditDefs(t, `{
				"cb-1": {"contexts": [
					"garbage",
					{"leafId": 20, "condition": {"country": "nz"}, "weights": [0, 1]}
				]}
			}`)))
		res := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, res.InExperiment())
		require.Equal(t, -1, *res.ExperimentResult.LeafId)
		require.Equal(t, []float64{0.5, 0.5}, res.ExperimentResult.VariationWeights)
	})

	t.Run("a junk banditVersion is omitted without dropping the definition", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
			WithContextualBandits(mustBanditDefs(t, `{
				"cb-1": {"banditVersion": "not-a-number", "contexts": [
					{"leafId": 10, "condition": {"country": "us"}, "weights": [1, 0]}
				]}
			}`)))
		res := client.EvalFeature(ctx, "bandit-flag")
		require.Equal(t, "a", res.Value)
		require.Equal(t, 10, *res.ExperimentResult.LeafId)
		require.Nil(t, res.ExperimentResult.BanditVersion)
	})

	t.Run("JS-falsy definitions dangle; truthy junk takes the fallback leaf", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
			WithContextualBandits(mustBanditDefs(t, `{"cb-1": null, "cb-truthy": 5}`)))

		// null definition == missing definition: plain experiment, no metadata.
		res := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, res.InExperiment())
		require.Nil(t, res.ExperimentResult.LeafId)
		require.Nil(t, res.ExperimentResult.VariationWeights)

		// A truthy non-object definition counts as found with no leaves:
		// fallback attribution.
		client2 := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
			WithContextualBandits(mustBanditDefs(t, `{"cb-1": 5}`)))
		res2 := client2.EvalFeature(ctx, "bandit-flag")
		require.True(t, res2.InExperiment())
		require.Equal(t, -1, *res2.ExperimentResult.LeafId)
		require.Equal(t, []float64{0.5, 0.5}, res2.ExperimentResult.VariationWeights)
	})

	t.Run("sticky-bucketed assignments carry no bandit fields", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
			WithStickyBucketService(NewInMemoryStickyBucketService()),
			WithDeferredTracking())

		first := client.EvalFeature(ctx, "bandit-flag")
		require.False(t, first.ExperimentResult.StickyBucketUsed)
		require.Equal(t, 10, *first.ExperimentResult.LeafId)
		client.ClearDeferredTrackingCalls()

		second := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, second.ExperimentResult.StickyBucketUsed)
		require.Nil(t, second.ExperimentResult.LeafId)
		require.Nil(t, second.ExperimentResult.VariationWeights)
		require.Nil(t, second.Experiment.ContextualBandit)

		calls := client.DeferredTrackingCalls()
		require.Len(t, calls, 1)
		require.Nil(t, calls[0].Experiment.ContextualBandit)
		require.Nil(t, calls[0].Result.LeafId)
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
			"null-force": {"defaultValue": "d", "rules": [{"force": null}]},
			"gated": {"defaultValue": "d", "rules": [{"force": "forced", "condition": {"country": "us"}}]}
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

	gated := reloaded.EvalFeature(ctx, "gated")
	require.Equal(t, "d", gated.Value, "rule conditions survive the round trip")

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

func TestContextualBanditRangesAndResync(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit ranges suppress bandit metadata; bucketing follows the ranges", func(t *testing.T) {
		// The leaf's weights [0, 1] would force variation 1; the explicit
		// ranges force variation 0. Step 9 buckets on ranges, so there is no
		// truthful propensity vector to report.
		features := `{"ranged": {"defaultValue": "default", "rules": [{
			"key": "ranged-exp",
			"contextualBanditRef": "cb-us-only",
			"contextualVariations": ["a", "b"],
			"ranges": [[0, 1], [0, 0]]
		}]}}`
		client, err := NewClient(ctx,
			WithJsonFeatures(features),
			WithContextualBandits(mustBanditDefs(t, `{
				"cb-us-only": {"contexts": [{"leafId": 10, "condition": {"country": "us"}, "weights": [0, 1]}]}
			}`)),
			WithAttributes(Attributes{"id": "u1", "country": "us"}))
		require.NoError(t, err)

		res := client.EvalFeature(ctx, "ranged")
		require.True(t, res.InExperiment())
		require.Equal(t, "a", res.Value, "ranges govern bucketing, not the leaf weights")
		require.Nil(t, res.ExperimentResult.LeafId)
		require.Nil(t, res.ExperimentResult.VariationWeights)
		require.Nil(t, res.ExperimentResult.BanditVersion)
		require.Nil(t, res.Experiment.ContextualBandit)
	})

	t.Run("inline experiments report the weights bucketing uses", func(t *testing.T) {
		client, err := NewClient(ctx, WithAttributes(Attributes{"id": "u1"}))
		require.NoError(t, err)

		two := 2
		exp := Experiment{
			Key:        "inline-bandit",
			Variations: []FeatureValue{"a", "b"},
			Weights:    []float64{1, 0},
			ContextualBandit: &ContextualBanditAssignment{
				LeafId:           7,
				VariationWeights: []float64{0, 1}, // inconsistent with Weights
				BanditVersion:    &two,
			},
		}
		res := client.RunExperiment(ctx, &exp)
		require.True(t, res.InExperiment)
		require.Equal(t, "a", res.Value)
		require.Equal(t, 7, *res.LeafId)
		require.Equal(t, []float64{1, 0}, res.VariationWeights,
			"reported propensities must be the weights bucketing used")

		// The caller's experiment is never mutated.
		require.Equal(t, []float64{0, 1}, exp.ContextualBandit.VariationWeights)
	})
}

func TestContextualBanditSubscriberVisibility(t *testing.T) {
	ctx := context.Background()

	subscribeBandit := func(t *testing.T, attrs Attributes, extra ...ClientOption) (*Client, *[]*Experiment) {
		t.Helper()
		var seen []*Experiment
		client := newBanditTestClient(t, attrs, extra...)
		client.Subscribe(func(_ context.Context, exp *Experiment, _ *ExperimentResult) {
			seen = append(seen, exp)
		})
		return client, &seen
	}

	t.Run("a real bandit assignment reaches subscribers with its context", func(t *testing.T) {
		client, seen := subscribeBandit(t, Attributes{"id": "u1", "country": "us"})
		client.EvalFeature(ctx, "bandit-flag")
		require.Len(t, *seen, 1)
		require.NotNil(t, (*seen)[0].ContextualBandit)
		require.Equal(t, 10, (*seen)[0].ContextualBandit.LeafId)
	})

	t.Run("a forced variation reaches subscribers with the context stripped", func(t *testing.T) {
		client, seen := subscribeBandit(t, Attributes{"id": "u1", "country": "us"},
			WithForcedVariations(ForcedVariationsMap{"bandit-exp": 1}))
		client.EvalFeature(ctx, "bandit-flag")
		require.Len(t, *seen, 1)
		require.Nil(t, (*seen)[0].ContextualBandit,
			"subscribers must see the same stripped experiment tracking records")
	})

	t.Run("a sticky-bucketed assignment reaches subscribers with the context stripped", func(t *testing.T) {
		client, seen := subscribeBandit(t, Attributes{"id": "u1", "country": "us"},
			WithStickyBucketService(NewInMemoryStickyBucketService()))
		client.EvalFeature(ctx, "bandit-flag")
		client.EvalFeature(ctx, "bandit-flag")
		require.Len(t, *seen, 1, "the unchanged second assignment does not re-notify")

		// Re-subscribe records the sticky-bucketed eval on a fresh client
		// sharing the same service via a child with the same attributes.
		client2, seen2 := subscribeBandit(t, Attributes{"id": "u1", "country": "us"},
			WithStickyBucketService(NewInMemoryStickyBucketService()))
		first := client2.EvalFeature(ctx, "bandit-flag")
		require.False(t, first.ExperimentResult.StickyBucketUsed)
		require.NotNil(t, (*seen2)[0].ContextualBandit)
	})
}

func TestContextualBanditTrackingRoundTrip(t *testing.T) {
	// detachTrackingData deep-copies via a JSON round trip — the mechanism
	// that silently lost Namespace in the deferred-tracking release. Pin that
	// bandit fields survive it, including the falsy-looking leafId values 0
	// and -1.
	ctx := context.Background()

	t.Run("a real leaf with leafId 0 survives the detach", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
			WithContextualBandits(mustBanditDefs(t, `{
				"cb-1": {"banditVersion": 0, "contexts": [
					{"leafId": 0, "condition": {"country": "us"}, "weights": [1, 0]}
				]}
			}`)),
			WithDeferredTracking())
		client.EvalFeature(ctx, "bandit-flag")

		calls := client.DeferredTrackingCalls()
		require.Len(t, calls, 1)
		require.Equal(t, 0, *calls[0].Result.LeafId)
		require.Equal(t, 0, *calls[0].Result.BanditVersion)
		require.NotNil(t, calls[0].Experiment.ContextualBandit)
		require.Equal(t, 0, calls[0].Experiment.ContextualBandit.LeafId)
		require.Equal(t, []float64{1, 0}, calls[0].Experiment.ContextualBandit.VariationWeights)

		b, err := json.Marshal(calls[0])
		require.NoError(t, err)
		var m map[string]map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &m))
		require.Equal(t, "0", string(m["result"]["leafId"]))
		require.Contains(t, m["experiment"], "contextualBandit")
		var cb ContextualBanditAssignment
		require.NoError(t, json.Unmarshal(m["experiment"]["contextualBandit"], &cb))
		require.Equal(t, 0, cb.LeafId)
		require.Equal(t, []float64{1, 0}, cb.VariationWeights)
		require.Equal(t, 0, *cb.BanditVersion)
	})

	t.Run("the fallback leaf -1 survives the detach", func(t *testing.T) {
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "de"}, WithDeferredTracking())
		client.EvalFeature(ctx, "bandit-no-match")

		calls := client.DeferredTrackingCalls()
		require.Len(t, calls, 1)
		require.Equal(t, -1, *calls[0].Result.LeafId)
		require.Equal(t, -1, calls[0].Experiment.ContextualBandit.LeafId)
	})
}

func TestContextualBanditPrerequisiteExposure(t *testing.T) {
	// A bandit rule deciding a prerequisite feature must land in the buffer
	// and callbacks with its real leaf attribution.
	ctx := context.Background()
	features := `{
		"parent-bandit": {"defaultValue": "default", "rules": [{
			"key": "parent-bandit-exp",
			"coverage": 1,
			"contextualBanditRef": "cb-us-only",
			"contextualVariations": ["a", "b"],
			"weights": [0.5, 0.5]
		}]},
		"gated-child": {"defaultValue": "off", "rules": [{
			"parentConditions": [{"id": "parent-bandit", "condition": {"value": "a"}}],
			"force": "on"
		}]}
	}`
	client, err := NewClient(ctx,
		WithJsonFeatures(features),
		WithContextualBandits(mustBanditDefs(t, banditDefsJSON)),
		WithAttributes(Attributes{"id": "u1", "country": "us"}),
		WithDeferredTracking())
	require.NoError(t, err)

	res := client.EvalFeature(ctx, "gated-child")
	require.Equal(t, "on", res.Value)

	calls := client.DeferredTrackingCalls()
	require.Len(t, calls, 1)
	require.Equal(t, "parent-bandit-exp", calls[0].Experiment.Key)
	require.Equal(t, "parent-bandit", calls[0].Result.FeatureId)
	require.Equal(t, 10, *calls[0].Result.LeafId)
	require.Equal(t, []float64{1, 0}, calls[0].Result.VariationWeights)
	require.Equal(t, 10, calls[0].Experiment.ContextualBandit.LeafId)
}

func TestContextualBanditPayloadSectionSemantics(t *testing.T) {
	// Absent section = preserve, explicit empty (or null) = clear, failed
	// decrypt = preserve — so a broken or bandit-less refresh never wipes a
	// coherent previous map (Python SDK parity).
	ctx := context.Background()

	banditAssigned := func(t *testing.T, client *Client) bool {
		t.Helper()
		res := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, res.InExperiment())
		return res.ExperimentResult.LeafId != nil
	}

	seed := func(t *testing.T, opts ...ClientOption) *Client {
		t.Helper()
		client, err := NewClient(ctx, append([]ClientOption{
			WithAttributes(Attributes{"id": "u1", "country": "us"}),
		}, opts...)...)
		require.NoError(t, err)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": `+banditFeaturesJSON+`,
			"contextualBandits": `+banditDefsJSON+`,
			"dateUpdated": "2030-01-01T00:00:00Z"
		}`))
		require.True(t, banditAssigned(t, client), "seed payload must assign a leaf")
		return client
	}

	t.Run("a refresh without the bandit section preserves the previous map", func(t *testing.T) {
		client := seed(t)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": `+banditFeaturesJSON+`,
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`))
		require.True(t, banditAssigned(t, client), "absent section must not clear definitions")
	})

	t.Run("an explicit empty section clears the map", func(t *testing.T) {
		client := seed(t)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": `+banditFeaturesJSON+`,
			"contextualBandits": {},
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`))
		require.False(t, banditAssigned(t, client), "explicit empty section must clear definitions")
	})

	t.Run("an explicit null section clears the map", func(t *testing.T) {
		client := seed(t)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": `+banditFeaturesJSON+`,
			"contextualBandits": null,
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`))
		require.False(t, banditAssigned(t, client))
	})

	t.Run("a failed bandit decryption preserves the previous map", func(t *testing.T) {
		client := seed(t, WithDecryptionKey("Ns04T5n9+59rl2x3SlNHtQ=="))
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": `+banditFeaturesJSON+`,
			"encryptedContextualBandits": "not-a-valid-blob",
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`))
		require.True(t, banditAssigned(t, client), "failed decrypt must not wipe the previous map")
	})
}

func TestBanditPropensitySlicesAreIndependent(t *testing.T) {
	// The result and the experiment's assignment reach independent consumers
	// (callbacks, subscribers, the caller); mutating one propensity slice
	// must not be visible through the other.
	ctx := context.Background()
	client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"})

	res := client.EvalFeature(ctx, "bandit-flag")
	require.Equal(t, []float64{1, 0}, res.ExperimentResult.VariationWeights)
	require.Equal(t, []float64{1, 0}, res.Experiment.ContextualBandit.VariationWeights)

	res.ExperimentResult.VariationWeights[0] = 99
	require.Equal(t, []float64{1, 0}, res.Experiment.ContextualBandit.VariationWeights,
		"result and assignment must not share a backing array")

	// Payload definitions are never aliased either.
	again := client.EvalFeature(ctx, "bandit-flag")
	require.Equal(t, []float64{1, 0}, again.ExperimentResult.VariationWeights)
}

func TestSemanticallyJunkConditionsRouteLikeJSAndPython(t *testing.T) {
	// A condition that parses but carries semantic junk evaluates false and
	// routing continues to the next leaf — verified byte-identical against
	// the Python SDK for all three cases (JS evaluates the same way).
	// Aborting instead would assign the same user different variations
	// across SDKs on the same payload.
	ctx := context.Background()
	for name, cond := range map[string]string{
		"junk $in argument": `{"country": {"$in": "not-an-array"}}`,
		"unknown operator":  `{"country": {"$frobnicate": "us"}}`,
		"invalid regex":     `{"country": {"$regex": "([invalid"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
				WithContextualBandits(mustBanditDefs(t, `{
					"cb-1": {"contexts": [
						{"leafId": 1, "condition": `+cond+`, "weights": [1, 0]},
						{"leafId": 2, "condition": {}, "weights": [0, 1]}
					]}
				}`)))
			res := client.EvalFeature(ctx, "bandit-flag")
			require.True(t, res.InExperiment())
			require.Equal(t, 2, *res.ExperimentResult.LeafId,
				"the junk condition evaluates false; the catch-all sibling matches")
			require.Equal(t, []float64{0, 1}, res.ExperimentResult.VariationWeights)
		})
	}
}

func TestInlineBanditWithoutWeightsReportsEqualWeights(t *testing.T) {
	// A caller-built experiment with bandit metadata but no Weights buckets
	// on equal weights; the reported propensities must say so, not echo the
	// caller's stale vector.
	ctx := context.Background()
	client, err := NewClient(ctx, WithAttributes(Attributes{"id": "u1"}))
	require.NoError(t, err)

	exp := Experiment{
		Key:        "inline-no-weights",
		Variations: []FeatureValue{"a", "b"},
		ContextualBandit: &ContextualBanditAssignment{
			LeafId:           7,
			VariationWeights: []float64{1, 0}, // stale: bucketing uses equal weights
		},
	}
	res := client.RunExperiment(ctx, &exp)
	require.True(t, res.InExperiment)
	require.Equal(t, []float64{0.5, 0.5}, res.VariationWeights,
		"reported propensities must be the equal weights bucketing used")
}

func TestBanditVersionPointersAreIndependent(t *testing.T) {
	// The version reaches three independently mutable owners: the client-wide
	// definitions map, the experiment's assignment, and the result. Writing
	// through one must not be visible through the others.
	ctx := context.Background()
	defs := mustBanditDefs(t, banditDefsJSON)
	client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
		WithContextualBandits(defs))

	res := client.EvalFeature(ctx, "bandit-flag")
	require.Equal(t, 3, *res.ExperimentResult.BanditVersion)

	*res.ExperimentResult.BanditVersion = 99
	require.Equal(t, 3, *res.Experiment.ContextualBandit.BanditVersion,
		"result must not share the assignment's pointer")
	def := defs["cb-1"]
	require.Equal(t, 3, *def.BanditVersion,
		"assignment must not share the definitions map's pointer")

	again := client.EvalFeature(ctx, "bandit-flag")
	require.Equal(t, 3, *again.ExperimentResult.BanditVersion,
		"later evaluations must be unaffected by consumer writes")
}

func TestBanditDefinitionsSurviveJSONRoundTrips(t *testing.T) {
	// Routing depends on tolerant-decoder state (malformed contexts abort
	// selection; falsy definitions dangle). Persisting or forwarding decoded
	// definitions as JSON must not change routing: marshaling emits the
	// original payload form.
	ctx := context.Background()

	roundTrip := func(t *testing.T, defs ContextualBanditDefinitions) ContextualBanditDefinitions {
		t.Helper()
		b, err := json.Marshal(defs)
		require.NoError(t, err)
		var back ContextualBanditDefinitions
		require.NoError(t, json.Unmarshal(b, &back))
		return back
	}

	leafOf := func(t *testing.T, defs ContextualBanditDefinitions) *int {
		t.Helper()
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "nz"},
			WithContextualBandits(defs))
		res := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, res.InExperiment())
		return res.ExperimentResult.LeafId
	}

	t.Run("a malformed context still aborts selection after a round trip", func(t *testing.T) {
		defs := mustBanditDefs(t, `{"cb-1": {"contexts": [
			"garbage",
			{"leafId": 20, "condition": {"country": "nz"}, "weights": [0, 1]}
		]}}`)
		require.Equal(t, -1, *leafOf(t, defs))
		require.Equal(t, -1, *leafOf(t, roundTrip(t, defs)),
			"round trip must not launder a malformed context into a valid leaf")
	})

	t.Run("a falsy definition still dangles after a round trip", func(t *testing.T) {
		defs := mustBanditDefs(t, `{"cb-1": null}`)
		require.Nil(t, leafOf(t, defs))
		require.Nil(t, leafOf(t, roundTrip(t, defs)),
			"round trip must not turn a falsy definition into a fallback definition")
	})

	t.Run("a junk banditVersion stays omitted after a round trip", func(t *testing.T) {
		defs := mustBanditDefs(t, `{"cb-1": {"banditVersion": "junk", "contexts": [
			{"leafId": 20, "condition": {"country": "nz"}, "weights": [0, 1]}
		]}}`)
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "nz"},
			WithContextualBandits(roundTrip(t, defs)))
		res := client.EvalFeature(ctx, "bandit-flag")
		require.Equal(t, 20, *res.ExperimentResult.LeafId)
		require.Nil(t, res.ExperimentResult.BanditVersion)
	})

	t.Run("programmatically built definitions marshal from their fields", func(t *testing.T) {
		three := 3
		ten := 10
		defs := ContextualBanditDefinitions{
			"cb-1": {BanditVersion: &three, Contexts: []ContextualBanditContext{
				{LeafId: &ten, Weights: []float64{0, 1}},
			}},
		}
		back := roundTrip(t, defs)
		client := newBanditTestClient(t, Attributes{"id": "u1", "country": "nz"},
			WithContextualBandits(back))
		res := client.EvalFeature(ctx, "bandit-flag")
		require.Equal(t, 10, *res.ExperimentResult.LeafId, "zero condition is a catch-all")
		require.Equal(t, []float64{0, 1}, res.ExperimentResult.VariationWeights)
		require.Equal(t, 3, *res.ExperimentResult.BanditVersion)
	})
}

func TestMutatedDecodedDefinitionsSerializeCurrentFields(t *testing.T) {
	// A caller who decodes definitions and updates fields must see those
	// updates in serialized output — evaluation and serialization always
	// agree on non-degenerate values.
	ctx := context.Background()
	defs := mustBanditDefs(t, `{"cb-1": {"banditVersion": 3, "contexts": [
		{"leafId": 10, "condition": {"country": "us"}, "weights": [1, 0]}
	]}}`)

	def := defs["cb-1"]
	def.Contexts[0].Weights = []float64{0, 1} // flip the leaf's weights
	*def.BanditVersion = 4

	b, err := json.Marshal(defs)
	require.NoError(t, err)
	var back ContextualBanditDefinitions
	require.NoError(t, json.Unmarshal(b, &back))

	client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
		WithContextualBandits(back))
	res := client.EvalFeature(ctx, "bandit-flag")
	require.Equal(t, "b", res.Value, "the mutated weights must govern after a round trip")
	require.Equal(t, []float64{0, 1}, res.ExperimentResult.VariationWeights)
	require.Equal(t, 4, *res.ExperimentResult.BanditVersion)
}

func TestExplicitNullLeafIdDemotes(t *testing.T) {
	// json.Unmarshal("null", &int) is a successful no-op; without an explicit
	// guard an explicit "leafId": null would attribute to leaf 0.
	ctx := context.Background()
	client := newBanditTestClient(t, Attributes{"id": "u1", "country": "us"},
		WithContextualBandits(mustBanditDefs(t, `{
			"cb-1": {"contexts": [{"leafId": null, "condition": {}, "weights": [1, 0]}]}
		}`)))
	res := client.EvalFeature(ctx, "bandit-flag")
	require.True(t, res.InExperiment())
	require.Equal(t, -1, *res.ExperimentResult.LeafId)
}
