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
			on, ok := jsonBool(v, "FeatureResult", "on")
			if !ok {
				return nil
			}
			result.On = on
		case "off":
			off, ok := jsonBool(v, "FeatureResult", "off")
			if !ok {
				return nil
			}
			result.Off = off
		case "source":
			source, ok := jsonString(v, "FeatureResult", "source")
			if !ok {
				return nil
			}
			result.Source = ParseFeatureResultSource(source)
		case "experiment":
			tmp, ok := v.(map[string]interface{})
			if !ok {
				logError("Invalid JSON data type", "FeatureResult", "experiment")
				continue
			}
			experiment := BuildExperiment(tmp)
			if experiment == nil {
				return nil
			}
			result.Experiment = experiment
		case "experimentResult":
			tmp, ok := v.(map[string]interface{})
			if !ok {
				logError("Invalid JSON data type", "FeatureResult", "experimentResult")
				return nil
			}
			experimentResult := BuildResult(tmp)
			if experimentResult == nil {
				return nil
			}
			result.ExperimentResult = experimentResult
		default:
			logWarn("Unknown key in JSON data", "FeatureResult", k)
		}
	}
	return &result
}
