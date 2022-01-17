package growthbook

import (
	"hash/fnv"
	"net/url"
	"strconv"
)

// VariationRange represents a single bucket range.
// TODO: PROPER DOCUMENTATION!
// TODO: DOES THIS NEED TO BE GLOBALLY ACCESSIBLE?
type VariationRange struct {
	Min float64
	Max float64
}

// getBucketRanges makes bucket ranges.
// TODO: PROPER DOCUMENTATION!
func getBucketRanges(numVariations int, coverage float64, weights []float64) []VariationRange {
	// Make sure coverage is within bounds
	if coverage < 0 {
		// log.Error("Experiment.coverage must be greater than or equal to 0")
		coverage = 0
	}
	if coverage > 1 {
		// log.Error("Experiment.coverage must be less than or equal to 1")
		coverage = 1
	}

	// Default to equal weights if missing or invalid
	equal := make([]float64, numVariations)
	for i := range equal {
		equal[i] = 1.0 / float64(numVariations)
	}
	if len(weights) == 0 {
		weights = equal
	}
	if len(weights) != numVariations {
		// log.Error("Experiment.weights array must be the same length as Experiment.variations")
		weights = equal
	}

	// If weights don't add up to 1 (or close to it), default to equal weights
	totalWeight := 0.0
	for i := range weights {
		totalWeight += weights[i]
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		// log.Error("Experiment.weights must add up to 1")
		weights = equal
	}

	// Convert weights to ranges
	cumulative := 0.0
	ranges := make([]VariationRange, len(weights))
	for i := range weights {
		start := cumulative
		cumulative += weights[i]
		ranges[i] = VariationRange{start, start + coverage*weights[i]}
	}
	return ranges
}

func chooseVariation(n float64, ranges []VariationRange) int {
	for i := range ranges {
		if n >= ranges[i].Min && n < ranges[i].Max {
			return i
		}
	}
	return -1
}

func getQueryStringOverride(id string, rawURL string, numVariations int) *int {
	if rawURL == "" {
		return nil
	}

	url, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	v, ok := url.Query()[id]
	if !ok || len(v) > 1 {
		return nil
	}

	vi, err := strconv.Atoi(v[0])
	if err != nil {
		return nil
	}

	if vi < 0 || vi > numVariations {
		return nil
	}

	return &vi
}

// Namespace specifies what part of a namespace an experiment
// includes. If two experiments are in the same namespace and their
// ranges don't overlap, they wil be mutually exclusive.
type Namespace struct {
	ID    string
	Start float64
	End   float64
}

func hashFnv32a(s string) uint32 {
	hash := fnv.New32a()
	hash.Write([]byte(s))
	return hash.Sum32()
}

func inNamespace(userID string, namespace *Namespace) bool {
	n := float64(hashFnv32a(userID+"__"+namespace.ID)%1000) / 1000
	return n >= namespace.Start && n < namespace.End
}
