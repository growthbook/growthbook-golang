package growthbook

import "encoding/json"

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
func getBucketRanges(numVariations int, coverage float64, weights []float64) []BucketRange {
	// Make sure coverage is within bounds.
	if coverage < 0 {
		logWarn("Experiment coverage must be greater than or equal to 0")
		coverage = 0
	}
	if coverage > 1 {
		logWarn("Experiment coverage must be less than or equal to 1")
		coverage = 1
	}

	// Default to equal weights if missing or invalid
	if weights == nil || len(weights) == 0 {
		weights = getEqualWeights(numVariations)
	}
	if len(weights) != numVariations {
		logWarn("Experiment weights and variations arrays must be the same length")
		weights = getEqualWeights(numVariations)
	}

	// If weights don't add up to 1 (or close to it), default to equal weights
	totalWeight := 0.0
	for i := range weights {
		totalWeight += weights[i]
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		logWarn("Experiment weights must add up to 1")
		weights = getEqualWeights(numVariations)
	}

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

// Given a hash and bucket ranges, assigns one of the bucket ranges.
func chooseVariation(n float64, ranges []BucketRange) int {
	for i := range ranges {
		if ranges[i].InRange(n) {
			return i
		}
	}
	return -1
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
