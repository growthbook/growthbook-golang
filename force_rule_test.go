package growthbook

import (
	"context"
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
