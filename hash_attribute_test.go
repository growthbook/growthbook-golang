package growthbook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Only a non-empty string or a finite, non-zero number is a usable hash
// attribute; anything else counts as missing and falls back (spec 0.12.0).
func TestHashAttributeUnusableValuesUseFallback(t *testing.T) {
	for name, id := range map[string]any{
		"true": true, "false": false, "array": []any{"1"}, "empty array": []any{},
		"object": map[string]any{}, "zero": 0, "empty string": "", "null": nil,
	} {
		t.Run(name, func(t *testing.T) {
			exp := Experiment{Key: "exp", Variations: []FeatureValue{0, 1}, FallbackAttribute: "deviceId"}
			attrs := Attributes{"id": id, "deviceId": "d1"}

			// Without sticky bucketing the fallback isn't used, as in JS
			c, err := NewClient(context.TODO(), WithAttributes(attrs), withSilentTestLogger())
			require.NoError(t, err)
			res := c.RunExperiment(context.TODO(), &exp)
			require.False(t, res.InExperiment)
			require.Equal(t, 0, res.Value)

			c, err = NewClient(context.TODO(), WithAttributes(attrs), withSilentTestLogger(),
				WithStickyBucketService(NewInMemoryStickyBucketService()))
			require.NoError(t, err)
			res = c.RunExperiment(context.TODO(), &exp)
			require.True(t, res.InExperiment)
			require.Equal(t, "deviceId", res.HashAttribute)
			require.Equal(t, "d1", res.HashValue)
		})
	}
}

func TestHashAttributeUnusableValuesWithoutFallbackSkip(t *testing.T) {
	for name, id := range map[string]any{"true": true, "array": []any{"1"}, "object": map[string]any{}} {
		t.Run(name, func(t *testing.T) {
			c, err := NewClient(context.TODO(), WithAttributes(Attributes{"id": id}), withSilentTestLogger(),
				WithStickyBucketService(NewInMemoryStickyBucketService()))
			require.NoError(t, err)
			res := c.RunExperiment(context.TODO(), &Experiment{Key: "exp", Variations: []FeatureValue{0, 1}})
			require.False(t, res.InExperiment)
			require.False(t, res.HashUsed)
		})
	}
}

// Numbers are hashed as JS String() formats them: 1.0 as "1".
func TestHashAttributeNumberFormatting(t *testing.T) {
	for id, want := range map[float64]string{1.0: "1", 1.5: "1.5", -3: "-3"} {
		c, err := NewClient(context.TODO(), WithAttributes(Attributes{"id": id}), withSilentTestLogger())
		require.NoError(t, err)
		res := c.RunExperiment(context.TODO(), &Experiment{Key: "exp", Variations: []FeatureValue{0, 1}})
		require.True(t, res.InExperiment)
		require.Equal(t, want, res.HashValue)
	}
}

// A feature is off only for null, false, 0 and "", as in JS.
func TestFeatureOnUsesJSTruthiness(t *testing.T) {
	for _, tt := range []struct {
		value string
		on    bool
	}{
		{`[]`, true}, {`{}`, true}, {`"off"`, true}, {`null`, false}, {`false`, false}, {`0`, false}, {`""`, false},
	} {
		features := FeatureMap{}
		require.NoError(t, json.Unmarshal([]byte(`{"f":{"defaultValue":`+tt.value+`}}`), &features))
		c, err := NewClient(context.TODO(), WithFeatures(features), withSilentTestLogger())
		require.NoError(t, err)
		res := c.EvalFeature(context.TODO(), "f")
		require.Equal(t, tt.on, res.On, tt.value)
		require.Equal(t, !tt.on, res.Off, tt.value)
	}
}

// Hashing uses UTF-16 code units, like JS charCodeAt.
func TestHashUsesUTF16CodeUnits(t *testing.T) {
	// U+1F600 is the surrogate pair D83D DE00
	require.Equal(t, hashFnv32a("\U0001F600"), fnvUnits(0xD83D, 0xDE00))
	require.Equal(t, hashFnv32a("é"), fnvUnits(0xE9))
	require.Equal(t, hashFnv32a("ab"), fnvUnits('a', 'b'))
}

func fnvUnits(units ...uint32) uint32 {
	h := uint32(0x811c9dc5)
	for _, u := range units {
		h = (h ^ u) * 0x01000193
	}
	return h
}
