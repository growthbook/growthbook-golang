package growthbook

import (
	"encoding/json"
	"regexp"

	"github.com/barkimedes/go-deepcopy"
)

// Attributes is an arbitrary JSON object containing user and request
// attributes.
type Attributes map[string]interface{}

func (attrs Attributes) fixSliceTypes() Attributes {
	fixed := Attributes{}
	for k, v := range attrs {
		fixed[k] = fixSliceTypes(v)
	}
	return fixed
}

// FeatureMap is a map of feature objects, keyed by string feature
// IDs.
type FeatureMap map[string]*Feature

func (fm FeatureMap) clone() FeatureMap {
	retval := FeatureMap{}
	for k, f := range fm {
		retval[k] = f.clone()
	}
	return retval
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

// ExperimentOverride provides the possibility to temporarily override
// some experiment settings.
type ExperimentOverride struct {
	Condition *Condition        `json:"condition,omitempty"`
	Weights   []float64         `json:"weights,omitempty"`
	Active    *bool             `json:"active,omitempty"`
	Status    *ExperimentStatus `json:"status,omitempty"`
	Force     *int              `json:"force,omitempty"`
	Coverage  *float64          `json:"coverage,omitempty"`
	Groups    []string          `json:"groups,omitempty"`
	Namespace *Namespace        `json:"namespace,omitempty"`
	URL       *regexp.Regexp    `json:"url,omitempty"`
}

func (o *ExperimentOverride) clone() *ExperimentOverride {
	retval := ExperimentOverride{}
	if o.Condition != nil {
		retval.Condition = deepcopy.MustAnything(o.Condition).(*Condition)
	}
	if o.Weights != nil {
		retval.Weights = make([]float64, len(o.Weights))
		copy(retval.Weights, o.Weights)
	}
	if o.Active != nil {
		tmp := *o.Active
		retval.Active = &tmp
	}
	if o.Status != nil {
		tmp := *o.Status
		retval.Status = &tmp
	}
	if o.Force != nil {
		tmp := *o.Force
		retval.Force = &tmp
	}
	if o.Coverage != nil {
		tmp := *o.Coverage
		retval.Coverage = &tmp
	}
	if o.Groups != nil {
		retval.Groups = make([]string, len(o.Groups))
		copy(retval.Groups, o.Groups)
	}
	if o.Namespace != nil {
		retval.Namespace = o.Namespace.clone()
	}
	if o.URL != nil {
		tmp := regexp.Regexp(*o.URL)
		retval.URL = &tmp
	}
	return &retval
}

type ExperimentOverrides map[string]*ExperimentOverride

func (os ExperimentOverrides) clone() ExperimentOverrides {
	retval := map[string]*ExperimentOverride{}
	for k, v := range os {
		retval[k] = v.clone()
	}
	return retval
}
