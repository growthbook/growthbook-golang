package growthbook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBucketRangesMatchJSRounding(t *testing.T) {
	client, err := NewClient(context.Background())
	require.NoError(t, err)

	ranges := client.getBucketRanges(5, 0.1, nil)
	require.Equal(t, 0.22000000000000003, ranges[1].Max)

	ranges = client.getBucketRanges(5, 0.72, nil)
	require.Equal(t, 0.344, ranges[1].Max)
}
