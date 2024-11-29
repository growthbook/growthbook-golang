package growthbook

// FeatureResult is the result of evaluating a feature.
type FeatureResult struct {
	RuleId           string
	Value            FeatureValue
	Source           FeatureResultSource
	On               bool
	Off              bool
	Experiment       *Experiment
	ExperimentResult *Result
}

// FeatureResultSource is an enumerated type representing the source
// of a FeatureResult.
type FeatureResultSource string

// FeatureResultSource values.
const (
	UnknownFeatureResultSource     FeatureResultSource = "unknownFeature"
	DefaultValueResultSource       FeatureResultSource = "defaultValue"
	ForceResultSource              FeatureResultSource = "force"
	ExperimentResultSource         FeatureResultSource = "experiment"
	OverrideResultSource           FeatureResultSource = "override"
	PrerequisiteResultSource       FeatureResultSource = "prerequisite"
	CyclicPrerequisiteResultSource FeatureResultSource = "cyclicPrerequisite"
)

func getFeatureResult(
	v FeatureValue,
	source FeatureResultSource,
	ruleId string,
	experiment *Experiment,
	experimentResult *Result,
) *FeatureResult {
	on := truthy(v)
	res := &FeatureResult{
		Value:            v,
		Source:           source,
		On:               on,
		Off:              !on,
		Experiment:       experiment,
		ExperimentResult: experimentResult,
		RuleId:           ruleId,
	}
	return res
}
