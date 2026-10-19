package growthbook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The expected ends are what the JavaScript and Python SDKs compute: the
// product is rounded, then the sum. They are written as literals because Go
// evaluates a constant expression exactly, which would hide the rounding.
//
// A build that fuses a multiply-add (arm64, or amd64 at GOAMD64=v3) rounds
// once and gets a different second end in both cases. On other builds these
// tests pass with or without the conversion in getBucketRanges.
func TestGetBucketRangesRoundsTheProductBeforeTheSum(t *testing.T) {
	fiveEqual := []float64{0.2, 0.2, 0.2, 0.2, 0.2}
	tests := []struct {
		name     string
		coverage float64
		want     []BucketRange
	}{
		{
			// Fused, the second end is 0.22.
			name:     "the end moves up",
			coverage: 0.1,
			want: []BucketRange{
				{0, 0.020000000000000004},
				{0.2, 0.22000000000000003},
				{0.4, 0.42000000000000004},
				{0.6000000000000001, 0.6200000000000001},
				{0.8, 0.8200000000000001},
			},
		},
		{
			// Fused, the second end is 0.34400000000000003.
			name:     "the end moves down",
			coverage: 0.72,
			want: []BucketRange{
				{0, 0.144},
				{0.2, 0.344},
				{0.4, 0.544},
				{0.6000000000000001, 0.7440000000000001},
				{0.8, 0.9440000000000001},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.want, client.getBucketRanges(5, tt.coverage, fiveEqual))
		})
	}
}

// user-3436 hashes to exactly 0.22 under this seed. The second variation's
// range is [0.2, 0.22000000000000003), so the user is in it.
func TestUserAtARangeEndGetsTheVariation(t *testing.T) {
	const features = `{"feature": {"defaultValue": "default", "rules": [{
        "key": "exp", "seed": "bucket-range-rounding", "hashVersion": 2, "hashAttribute": "id",
        "coverage": 0.1, "variations": ["v0", "v1", "v2", "v3", "v4"], "weights": [0.2, 0.2, 0.2, 0.2, 0.2]
    }]}}`
	require.Equal(t, 0.22, *hash("bucket-range-rounding", "user-3436", 2))

	var fm FeatureMap
	require.NoError(t, json.Unmarshal([]byte(features), &fm))
	client, err := NewClient(context.Background(), WithFeatures(fm))
	require.NoError(t, err)
	user, err := client.WithAttributes(Attributes{"id": "user-3436"})
	require.NoError(t, err)

	res := user.EvalFeature(context.Background(), "feature")
	require.NotNil(t, res)
	require.Equal(t, "v1", res.Value)
}
