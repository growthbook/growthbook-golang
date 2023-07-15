package growthbook

import (
	"encoding/json"
	"errors"
)

// Range represents a single bucket range.
type Range struct {
	Min float64
	Max float64
}

func (r Range) MarshalJSON() ([]byte, error) {
	return json.Marshal([]float64{r.Min, r.Max})
}

func (r *Range) UnmarshalJSON(data []byte) error {
	tmp := []float64{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	if len(tmp) != 2 {
		return errors.New("invalid array for range")
	}
	r.Min = tmp[0]
	r.Max = tmp[1]
	return nil
}

func (r Range) InRange(n float64) bool {
	return n >= r.Min && n < r.Max
}

// This converts an experiment's coverage and variation weights into
// an array of bucket ranges.
func getBucketRanges(numVariations int, coverage float64, weights []float64) []Range {
	// Make sure coverage is within bounds.
	if coverage < 0 {
		logWarn(CoverageOutOfRange, nil)
		coverage = 0
	}
	if coverage > 1 {
		logWarn(CoverageOutOfRange, nil)
		coverage = 1
	}

	// Default to equal weights if missing or invalid
	if weights == nil || len(weights) == 0 {
		weights = getEqualWeights(numVariations)
	}
	if len(weights) != numVariations {
		logWarn(ExperimentWeightVariationLengthMismatch, nil)
		weights = getEqualWeights(numVariations)
	}

	// If weights don't add up to 1 (or close to it), default to equal weights
	totalWeight := 0.0
	for i := range weights {
		totalWeight += weights[i]
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		logWarn(ExperimentWeightsTotal, nil)
		weights = getEqualWeights(numVariations)
	}

	// Convert weights to ranges
	cumulative := 0.0
	ranges := make([]Range, len(weights))
	for i := range weights {
		start := cumulative
		cumulative += weights[i]
		ranges[i] = Range{start, start + coverage*weights[i]}
	}
	return ranges
}

// Given a hash and bucket ranges, assigns one of the bucket ranges.
func chooseVariation(n float64, ranges []Range) int {
	for i := range ranges {
		if ranges[i].InRange(n) {
			return i
		}
	}
	return -1
}
