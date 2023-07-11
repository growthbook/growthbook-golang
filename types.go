package growthbook

import (
	"encoding/json"

	"github.com/barkimedes/go-deepcopy"
)

// Attributes is an arbitrary JSON object containing user and request
// attributes.
type Attributes map[string]interface{}

// FeatureMap is a map of feature objects, keyed by string feature
// IDs.
type FeatureMap map[string]*Feature

// ForcedVariationsMap is a map that forces an Experiment to always
// assign a specific variation. Useful for QA.
//
// Keys are the experiment key, values are the array index of the
// variation.
type ForcedVariationsMap map[string]int

func (fv ForcedVariationsMap) Copy() ForcedVariationsMap {
	return deepcopy.MustAnything(fv).(ForcedVariationsMap)
}

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

func (s FeatureResultSource) MarshalJSON() ([]byte, error) {
	switch s {
	case DefaultValueResultSource:
		return []byte("defaultValue"), nil
	case ForceResultSource:
		return []byte("force"), nil
	case ExperimentResultSource:
		return []byte("experiment"), nil
	case OverrideResultSource:
		return []byte("override"), nil
	default:
		return []byte("unknown"), nil
	}
}

func (s *FeatureResultSource) UnmarshalJSON(data []byte) error {
	val := ""
	err := json.Unmarshal(data, &val)
	if err != nil {
		return err
	}
	switch val {
	case "", "defaultValue":
		*s = DefaultValueResultSource
	case "force":
		*s = ForceResultSource
	case "experiment":
		*s = ExperimentResultSource
	case "override":
		*s = OverrideResultSource
	default:
		*s = UnknownResultSource
	}
	return nil
}
