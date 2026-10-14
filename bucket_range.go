package growthbook

import (
	"encoding/json"
	"log/slog"
	"math"
)

// BucketRange represents a single bucket range.
type BucketRange struct {
	Min float64
	Max float64
}

func (r *BucketRange) InRange(n float64) bool {
	return n >= r.Min && n < r.Max
}

// This converts an experiment's coverage and variation weights into
// an array of bucket ranges.
func (c *Client) getBucketRanges(numVariations int, coverage float64, weights []float64) []BucketRange {
	// Make sure coverage is within bounds.
	if coverage < 0 {
		c.logger.Warn("Experiment coverage must be greater than or equal to 0")
		coverage = 0
	}
	if coverage > 1 {
		c.logger.Warn("Experiment coverage must be less than or equal to 1")
		coverage = 1
	}

	weights = normalizedWeights(numVariations, weights, c.logger)

	// Cast weights to ranges
	cumulative := 0.0
	ranges := make([]BucketRange, len(weights))
	for i := range weights {
		start := cumulative
		cumulative += weights[i]
		ranges[i] = BucketRange{start, start + coverage*weights[i]}
	}
	return ranges
}

// isValidWeightVector reports whether weights is a usable propensity vector
// for numVariations variations: right length, every element finite and
// non-negative, and summing to ~1. Deliberately stricter than the JS SDK,
// which checks only length and sum and buckets on inverted ranges for
// vectors like [1.2, -0.2]; the Python SDK applies this same rule.
func isValidWeightVector(weights []float64, numVariations int) bool {
	if len(weights) == 0 || len(weights) != numVariations {
		return false
	}
	total := 0.0
	for _, w := range weights {
		if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
			return false
		}
		total += w
	}
	return total >= 0.99 && total <= 1.01
}

// normalizedWeights returns the weights bucketing will actually use: the
// input when it is a valid vector, equal weights otherwise. Reported bandit
// propensities come from the same function, so they always describe the
// vector bucketing used. A nil logger skips the warnings.
func normalizedWeights(numVariations int, weights []float64, logger *slog.Logger) []float64 {
	if len(weights) == 0 {
		return getEqualWeights(numVariations)
	}
	if isValidWeightVector(weights, numVariations) {
		return weights
	}
	if logger != nil {
		if len(weights) != numVariations {
			logger.Warn("Experiment weights and variations arrays must be the same length")
		} else {
			logger.Warn("Experiment weights must be finite, non-negative, and add up to 1")
		}
	}
	return getEqualWeights(numVariations)
}

// Given a hash and bucket ranges, assigns one of the bucket ranges.
func chooseVariation(n float64, ranges []BucketRange) int {
	for i := range ranges {
		if ranges[i].InRange(n) {
			return i
		}
	}
	return -1
}

// Returns an array of floats with numVariations items that are all
// equal and sum to 1.
func getEqualWeights(numVariations int) []float64 {
	if numVariations < 0 {
		numVariations = 0
	}
	equal := make([]float64, numVariations)
	for i := range equal {
		equal[i] = 1.0 / float64(numVariations)
	}
	return equal
}

func (br *BucketRange) UnmarshalJSON(data []byte) error {
	var pair [2]float64
	err := json.Unmarshal(data, &pair)
	if err != nil {
		return err
	}
	br.Min = float64(pair[0])
	br.Max = float64(pair[1])
	return nil
}

func (br BucketRange) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]float64{br.Min, br.Max})
}
