package growthbook

// FeatureResult is the result of evaluating a feature.
type FeatureResult struct {
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
	UnknownFeatureResultSource FeatureResultSource = "unknownFeature"
	DefaultValueResultSource   FeatureResultSource = "defaultValue"
	ForceResultSource          FeatureResultSource = "force"
	ExperimentResultSource     FeatureResultSource = "experiment"
)

func getFeatureResult(value FeatureValue, source FeatureResultSource,
	experiment *Experiment, experimentResult *Result) *FeatureResult {
	on := truthy(value)
	res := &FeatureResult{
		Value:            value,
		Source:           source,
		On:               on,
		Off:              !on,
		Experiment:       experiment,
		ExperimentResult: experimentResult,
	}
	return res
}

// This function imitates Javascript's "truthiness" evaluation for Go
// values of unknown type.
func truthy(v interface{}) bool {
	if v == nil {
		return false
	}
	switch v.(type) {
	case string:
		return v.(string) != ""
	case bool:
		return v.(bool)
	case int:
		return v.(int) != 0
	case uint:
		return v.(uint) != 0
	case float32:
		return v.(float32) != 0
	case float64:
		return v.(float64) != 0
	}
	return true
}
