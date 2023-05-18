package growthbook

// Range represents a single bucket range.
type Range struct {
	Min float64
	Max float64
}

func (r *Range) InRange(n float64) bool {
	return n >= r.Min && n < r.Max
}

// This converts an experiment's coverage and variation weights into
// an array of bucket ranges.
func getBucketRanges(numVariations int, coverage float64, weights []float64) []Range {
	// Make sure coverage is within bounds.
	if coverage < 0 {
		logWarn(WarnExpCoverageMustBePositive)
		coverage = 0
	}
	if coverage > 1 {
		logWarn(WarnExpCoverageMustBeFraction)
		coverage = 1
	}

	// Default to equal weights if missing or invalid
	if weights == nil || len(weights) == 0 {
		weights = getEqualWeights(numVariations)
	}
	if len(weights) != numVariations {
		logWarn(WarnExpWeightsWrongLength)
		weights = getEqualWeights(numVariations)
	}

	// If weights don't add up to 1 (or close to it), default to equal weights
	totalWeight := 0.0
	for i := range weights {
		totalWeight += weights[i]
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		logWarn(WarnExpWeightsWrongTotal)
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
		if n >= ranges[i].Min && n < ranges[i].Max {
			return i
		}
	}
	return -1
}

func jsonRange(v interface{}, typeName string, fieldName string) *Range {
	vals := jsonFloatArray(v, typeName, fieldName)
	if vals == nil || len(vals) != 2 {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}
	return &Range{vals[0], vals[1]}
}

func jsonRangeArray(v interface{}, typeName string, fieldName string) []Range {
	vals, ok := v.([]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}
	ranges := make([]Range, len(vals))
	for i := range vals {
		tmp := jsonRange(vals[i], typeName, fieldName)
		if tmp == nil {
			return nil
		}
		ranges[i] = *tmp
	}
	return ranges
}
