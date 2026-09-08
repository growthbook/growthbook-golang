package growthbook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForceNullRule(t *testing.T) {
	ctx := context.Background()
	client, err := NewClient(ctx,
		WithJsonFeatures(`{
		  "forced-null": {
		    "defaultValue": "default",
		    "rules": [{"force": null}, {"force": "second-rule"}]
		  },
		  "gated-null": {
		    "defaultValue": "default",
		    "rules": [
		      {"force": null, "condition": {"country": "us"}},
		      {"force": "second-rule"}
		    ]
		  },
		  "empty-rule": {
		    "defaultValue": "default",
		    "rules": [{}]
		  }
		}`),
		WithAttributes(Attributes{"id": "u1", "country": "nz"}),
	)
	require.NoError(t, err)

	t.Run("a null force serves null (JS parity)", func(t *testing.T) {
		res := client.EvalFeature(ctx, "forced-null")
		require.Nil(t, res.Value)
		require.Equal(t, ForceResultSource, res.Source)
		require.True(t, res.Off)
	})

	t.Run("a null force still honors its condition", func(t *testing.T) {
		res := client.EvalFeature(ctx, "gated-null")
		require.Equal(t, "second-rule", res.Value)
	})

	t.Run("an absent force is not a force rule", func(t *testing.T) {
		res := client.EvalFeature(ctx, "empty-rule")
		require.Equal(t, "default", res.Value)
		require.Equal(t, DefaultValueResultSource, res.Source)
	})
}

func TestFeatureRuleForceRoundTrip(t *testing.T) {
	t.Run("an absent force stays absent through a marshal round trip", func(t *testing.T) {
		var rule FeatureRule
		require.NoError(t, json.Unmarshal([]byte(`{"variations": ["a", "b"], "weights": [1, 0], "coverage": 1}`), &rule))
		require.False(t, rule.forcePresent)

		b, err := json.Marshal(rule)
		require.NoError(t, err)
		require.NotContains(t, string(b), `"force"`)

		var back FeatureRule
		require.NoError(t, json.Unmarshal(b, &back))
		require.False(t, back.forcePresent,
			"a round trip must not turn a variations rule into a forced-null rule")
	})

	t.Run("an explicit null force survives a marshal round trip", func(t *testing.T) {
		var rule FeatureRule
		require.NoError(t, json.Unmarshal([]byte(`{"force": null}`), &rule))
		require.True(t, rule.forcePresent)

		b, err := json.Marshal(rule)
		require.NoError(t, err)
		require.Contains(t, string(b), `"force":null`)

		var back FeatureRule
		require.NoError(t, json.Unmarshal(b, &back))
		require.True(t, back.forcePresent)
	})
}
