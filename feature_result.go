package growthbook

// FeatureResult is the result of evaluating a feature.
type FeatureResult struct {
	RuleId           string              `json:"ruleId"`
	Value            FeatureValue        `json:"value"`
	Source           FeatureResultSource `json:"source"`
	On               bool                `json:"on"`
	Off              bool                `json:"off"`
	Experiment       *Experiment         `json:"experiment"`
	ExperimentResult *ExperimentResult   `json:"experimentResult"`
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
	experimentResult *ExperimentResult,
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

func (res *FeatureResult) InExperiment() bool {
	return res.Experiment != nil &&
		res.ExperimentResult != nil &&
		res.ExperimentResult.InExperiment
}
