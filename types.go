package growthbook

// Attributes is an arbitrary JSON object containiner user and request
// attributes.
type Attributes map[string]interface{}

// Condition ...
type Condition interface {
	Eval(attrs Attributes) bool
}

// Context contains the options for creating a new GrowthBook
// instance.
type Context struct {
	Enabled    bool
	Attributes Attributes
	// TODO: USE GO'S URL TYPE?
	URL              *string
	Features         FeatureMap
	ForcedVariations ForcedVariationsMap
	QaMode           bool
	TrackingCallback TrackingCallback
}

// Experiment defines a single experiment.
type Experiment struct {
	Key           string
	Variations    []interface{}
	Weights       []float64
	Active        bool
	Coverage      *float64
	Condition     Condition
	Namespace     *Namespace
	Force         *int
	HashAttribute *string
}

// IF experiment.Variations HAS TYPE []T, THEN Run(experiment) SHOULD
// RETURN A RESULT WITH A Value OF TYPE T. GENERICS WOULD BE NICE, BUT
// THEY'RE ONLY COMING IN Go 1.18.

// ExperimentResult records the result of running an Experiment given
// a specific Context.
type ExperimentResult struct {
	InExperiment  bool
	VariationID   int
	Value         interface{}
	HashAttribute string
	HashValue     string
}

// Feature has a default value plus rules than can override the
// default.
type Feature struct {
	DefaultValue interface{}
	Rules        []*FeatureRule
}

// FeatureMap is a map of feature objects, keyed by string feature
// IDs.
type FeatureMap map[string]*Feature

// FeatureResultSource represents the source of a FeatureResult.
type FeatureResultSource uint

// FeatureResultSource values.
const (
	UnknownFeatureResultSource FeatureResultSource = iota + 1
	DefaultValueResultSource
	ForceResultSource
	ExperimentResultSource
)

// ParseFeatureResultSource ...
func ParseFeatureResultSource(source string) FeatureResultSource {
	switch source {
	case "defaultValue":
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
	Value            interface{}
	On               bool
	Off              bool
	Source           FeatureResultSource
	Experiment       *Experiment
	ExperimentResult *ExperimentResult
}

// FeatureRule overrides the default value of a Feature.
type FeatureRule struct {
	Condition     Condition
	Coverage      *float64
	Force         interface{}
	Variations    []interface{}
	TrackingKey   *string
	Weights       []float64
	Namespace     *Namespace
	HashAttribute *string
}

// ForcedVariationsMap is a map that forces an Experiment to always
// assign a specific variation. Useful for QA.
//
// Keys are the experiment key, values are the array index of the
// variation.
type ForcedVariationsMap map[string]int

// Namespace specifies what part of a namespace an experiment
// includes. If two experiments are in the same namespace and their
// ranges don't overlap, they wil be mutually exclusive.
type Namespace struct {
	ID    string
	Start float64
	End   float64
}

// TrackingCallback is a callback function that is executed every time
// a user is included in an Experiment.
type TrackingCallback func(experiment *Experiment, result *ExperimentResult)

// VariationRange represents a single bucket range.
type VariationRange struct {
	Min float64
	Max float64
}
