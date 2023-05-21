package growthbook

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

// BuildFeatureResult creates an FeatureResult value from a JSON
// object represented as a Go map.
func BuildFeatureResult(dict map[string]interface{}) *FeatureResult {
	result := FeatureResult{}
	for k, v := range dict {
		switch k {
		case "value":
			result.Value = v
		case "on":
			result.On = jsonBool(v, "FeatureResult", "on")
		case "off":
			result.Off = jsonBool(v, "FeatureResult", "off")
		case "source":
			result.Source = ParseFeatureResultSource(jsonString(v, "FeatureResult", "source"))
		case "experiment":
			tmp, ok := v.(map[string]interface{})
			if !ok {
				logError("Invalid JSON data type", "FeatureResult", "experiment")
				continue
			}
			result.Experiment = BuildExperiment(tmp)
		case "experimentResult":
			tmp, ok := v.(map[string]interface{})
			if !ok {
				logError("Invalid JSON data type", "FeatureResult", "experimentResult")
				continue
			}
			result.ExperimentResult = BuildResult(tmp)
		default:
			logWarn("Unknown key in JSON data", "FeatureResult", k)
		}
	}
	return &result
}
