package growthbook

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetBucketRangesWarnsAndFallsBackForInvalidInputs(t *testing.T) {
	tests := []struct {
		name     string
		num      int
		coverage float64
		weights  []float64
		want     []BucketRange
		warnings []string
	}{
		{
			name:     "negative coverage",
			num:      2,
			coverage: -0.2,
			want:     []BucketRange{{0, 0}, {0.5, 0.5}},
			warnings: []string{"Experiment coverage must be greater than or equal to 0"},
		},
		{
			name:     "coverage above one",
			num:      2,
			coverage: 1.5,
			want:     []BucketRange{{0, 0.5}, {0.5, 1}},
			warnings: []string{"Experiment coverage must be less than or equal to 1"},
		},
		{
			name:     "weights length mismatch",
			num:      4,
			coverage: 1,
			weights:  []float64{0.4, 0.4, 0.2},
			want:     []BucketRange{{0, 0.25}, {0.25, 0.5}, {0.5, 0.75}, {0.75, 1}},
			warnings: []string{"Experiment weights and variations arrays must be the same length"},
		},
		{
			name:     "weights sum below one",
			num:      2,
			coverage: 1,
			weights:  []float64{0.4, 0.1},
			want:     []BucketRange{{0, 0.5}, {0.5, 1}},
			warnings: []string{"Experiment weights must add up to 1"},
		},
		{
			name:     "valid approximate weights",
			num:      2,
			coverage: 1,
			weights:  []float64{0.4, 0.5999},
			want:     []BucketRange{{0, 0.4}, {0.4, 0.9999}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, logs := testLogger(slog.LevelWarn, t)
			client, err := NewClient(context.Background(), WithLogger(logger))
			require.NoError(t, err)

			got := client.getBucketRanges(tt.num, tt.coverage, tt.weights)
			require.Equal(t, tt.want, roundRanges(got))
			require.Len(t, *logs, len(tt.warnings))
			for i, warning := range tt.warnings {
				require.Equal(t, warning, (*logs)[i].Message)
			}
		})
	}
}
