package growthbook

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChildClient(t *testing.T) {
	ctx := context.TODO()
	client, _ := NewClient(ctx,
		WithEnabled(false),
		WithQaMode(false),
		WithAttributes(Attributes{"user": 1}),
	)
	t.Run("WithAttributes", func(t *testing.T) {
		child, _ := client.WithAttributes(Attributes{"user": 2})
		require.Equal(t, client.attributes, Attributes{"user": 1})
		require.Equal(t, child.attributes, Attributes{"user": 2})
	})

	t.Run("WithEnabled", func(t *testing.T) {
		child, _ := client.WithEnabled(true)
		require.False(t, client.enabled)
		require.True(t, child.enabled)
	})

	t.Run("WithQaMode", func(t *testing.T) {
		child, _ := client.WithQaMode(true)
		require.False(t, client.qaMode)
		require.True(t, child.qaMode)
	})
}

func TestClientEvalFeatures(t *testing.T) {
	features := FeatureMap{"feature": &Feature{DefaultValue: FeatureValue(0)}}
	ctx := context.TODO()
	client, _ := NewClient(ctx, WithFeatures(features))

	t.Run("unknown feature", func(t *testing.T) {
		result := client.EvalFeature(ctx, "unknown")
		expected := &FeatureResult{
			Value:  nil,
			On:     false,
			Off:    true,
			Source: UnknownFeatureResultSource,
		}
		require.Equal(t, result, expected)
	})

	t.Run("feature default value", func(t *testing.T) {
		result := client.EvalFeature(ctx, "feature")
		expected := &FeatureResult{
			Value:  0,
			On:     false,
			Off:    true,
			Source: DefaultValueResultSource,
		}
		require.Equal(t, result, expected)
	})
}
