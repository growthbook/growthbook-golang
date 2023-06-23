package growthbook

import "encoding/json"

// Attributes is an arbitrary JSON object containing user and request
// attributes.
type Attributes map[string]interface{}

// FeatureMap is a map of feature objects, keyed by string feature
// IDs.
type FeatureMap map[string]*Feature

// ParseFeatureMap creates a FeatureMap value from raw JSON input.
func ParseFeatureMap(data []byte) FeatureMap {
	dict := map[string]interface{}{}
	err := json.Unmarshal(data, &dict)
	if err != nil {
		logError("Failed parsing JSON input", "FeatureMap")
		return nil
	}
	return BuildFeatureMap(dict)
}

// BuildFeatureMap creates a FeatureMap value from a JSON object
// represented as a Go map.
func BuildFeatureMap(dict map[string]interface{}) FeatureMap {
	fmap := FeatureMap{}
	for k, v := range dict {
		feature := BuildFeature(v)
		if feature != nil {
			fmap[k] = BuildFeature(v)
		}
	}
	return fmap
}

// ForcedVariationsMap is a map that forces an Experiment to always
// assign a specific variation. Useful for QA.
//
// Keys are the experiment key, values are the array index of the
// variation.
type ForcedVariationsMap map[string]int

// URL matching supports regular expressions or simple string matches.
type URLTargetType uint

const (
	RegexURLTarget  URLTargetType = iota
	SimpleURLTarget               = iota
)

// URL match target.
type URLTarget struct {
	Type    URLTargetType
	Include bool
	Pattern string
}

// FeatureResultSource is an enumerated type representing the source
// of a FeatureResult.
type FeatureResultSource uint

// FeatureResultSource values.
const (
	UnknownResultSource FeatureResultSource = iota + 1
	DefaultValueResultSource
	ForceResultSource
	ExperimentResultSource
	OverrideResultSource
)

// ParseFeatureResultSource creates a FeatureResultSource value from
// its string representation.
func ParseFeatureResultSource(source string) FeatureResultSource {
	switch source {
	case "", "defaultValue":
		return DefaultValueResultSource
	case "force":
		return ForceResultSource
	case "experiment":
		return ExperimentResultSource
	case "override":
		return OverrideResultSource
	default:
		return UnknownResultSource
	}
}
