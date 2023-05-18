package growthbook

// Attributes is an arbitrary JSON object containing user and request
// attributes.
type Attributes map[string]interface{}

// FeatureValue is a wrapper around an arbitrary type representing the
// value of a feature. Features can return any kinds of values, so
// this is an alias for interface{}.
type FeatureValue interface{}

// Feature has a default value plus rules than can override the
// default.
type Feature struct {
	DefaultValue FeatureValue
	Rules        []*FeatureRule
}

// FeatureMap is a map of feature objects, keyed by string feature
// IDs.
type FeatureMap map[string]*Feature

// FeatureResultSource is an enumerated type representing the source
// of a FeatureResult.
type FeatureResultSource uint

// FeatureResultSource values.
const (
	UnknownFeatureResultSource FeatureResultSource = iota + 1
	DefaultValueResultSource
	ForceResultSource
	ExperimentResultSource
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
	default:
		return UnknownFeatureResultSource
	}
}

// FeatureResult is the result of evaluating a feature.
type FeatureResult struct {
	Value            FeatureValue
	Source           FeatureResultSource
	On               bool
	Off              bool
	RuleID           string
	Experiment       *Experiment
	ExperimentResult *Result
}

// Result records the result of running an Experiment given a specific
// Context.
type Result struct {
	Value       FeatureValue
	VariationID int
	Key         string
	Name        string
	Bucket      *float64
	// Passthrough
	InExperiment  bool
	HashUsed      bool
	HashAttribute string
	HashValue     string
	FeatureID     string
}

// Experiment defines a single experiment.
type Experiment struct {
	Key        string
	Variations []FeatureValue
	Ranges     []Range
	Meta       []VariationMeta
	// Filters
	Seed  string
	Name  string
	Phase string
	// URLPatterns
	Weights   []float64
	Condition Condition
	Coverage  *float64
	// Include?
	Namespace     *Namespace
	Force         *int
	Active        bool
	HashAttribute string
	HashVersion   int
	// Status
	// URL
	// Groups
}

// VariationMeta represents meta-information that can be passed
// through to tracking callbacks.
type VariationMeta struct {
	Passthrough bool
	Key         string
	Name        string
}

// Range is used to express the traffic split ranges.
type Range struct {
	Low  float64
	High float64
}

func (r *Range) InRange(n float64) bool {
	return n >= r.Low && n < r.High
}

// FeatureRule overrides the default value of a Feature.
type FeatureRule struct {
	ID            string
	Condition     Condition
	Force         FeatureValue
	Variations    []FeatureValue
	Weights       []float64
	Key           string
	HashAttribute string
	HashVersion   int
	Range         *Range
	Coverage      *float64
	Namespace     *Namespace
	Ranges        []Range
	Meta          []VariationMeta
	Seed          string
	Name          string
	Phase         string

	// TBD:
	// Filters
	// Tracks
}

// ForcedVariationsMap is a map that forces an Experiment to always
// assign a specific variation. Useful for QA.
//
// Keys are the experiment key, values are the array index of the
// variation.
type ForcedVariationsMap map[string]int
