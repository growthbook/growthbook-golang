package growthbook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
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
		require.Equal(t, client.attributes, value.ObjValue{"user": value.Num(1)})
		require.Equal(t, child.attributes, value.ObjValue{"user": value.Num(2)})
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
	features := FeatureMap{"feature": &Feature{DefaultValue: 0}}
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
		require.Equal(t, expected, result)
	})
}

func TestClientSetFeatures(t *testing.T) {
	ctx := context.TODO()
	client, _ := NewClient(ctx, WithAttributes(Attributes{"id": "123"}))
	client.SetFeatures(FeatureMap{"feature": &Feature{DefaultValue: 0}})

	result := client.EvalFeature(ctx, "feature")
	expected := &FeatureResult{
		Value:  0,
		On:     false,
		Off:    true,
		Source: DefaultValueResultSource,
	}

	require.Equal(t, result, expected)
}

func TestClientSetJSONFeatures(t *testing.T) {
	ctx := context.TODO()
	client, _ := NewClient(ctx, WithAttributes(Attributes{"id": "123"}))
	featuresJSON := `{"feature1": {"defaultValue": 0}}`
	err := client.SetJSONFeatures(featuresJSON)
	require.Nil(t, err)
	expected := FeatureMap{
		"feature1": &Feature{DefaultValue: 0.0},
	}
	require.Equal(t, client.data.features, expected)
}

func TestClientSetEncryptedJSONFeatures(t *testing.T) {
	key := "Ns04T5n9+59rl2x3SlNHtQ=="
	ctx := context.TODO()
	client, _ := NewClient(ctx, WithDecryptionKey(key))

	encryptedFeatures :=
		"vMSg2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	err := client.SetEncryptedJSONFeatures(encryptedFeatures)
	require.Nil(t, err)

	expectedJSON := `{
    "testfeature1": {
        "defaultValue": true,
        "rules": [{"condition": { "id": "1234" }, "force": false}]
      }
    }`

	var expected FeatureMap
	err = json.Unmarshal([]byte(expectedJSON), &expected)
	require.Nil(t, err)
	require.Equal(t, client.data.features, expected)
}
