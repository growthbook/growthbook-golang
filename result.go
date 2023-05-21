package growthbook

// Result records the result of running an Experiment given a specific
// Context.
type Result struct {
	Value         FeatureValue
	VariationID   int
	Key           string
	Name          string
	Bucket        *float64
	Passthrough   bool
	InExperiment  bool
	HashUsed      bool
	HashAttribute string
	HashValue     string
	FeatureID     string
}

// BuildResult creates an Result value from a JSON object represented
// as a Go map.
func BuildResult(dict map[string]interface{}) *Result {
	res := Result{}
	for k, v := range dict {
		switch k {
		case "value":
			res.Value = v
		case "variationId":
			res.VariationID = jsonInt(v, "Result", "variationId")
		case "inExperiment":
			res.InExperiment = jsonBool(v, "Result", "inExperiment")
		case "hashUsed":
			res.HashUsed = jsonBool(v, "Result", "hashUsed")
		case "hashAttribute":
			res.HashAttribute = jsonString(v, "Result", "hashAttribute")
		case "hashValue":
			tmp, ok := convertHashValue(v)
			if !ok {
				logError("Invalid JSON data type", "Result", "hashValue")
				continue
			}
			res.HashValue = tmp
		case "featureId":
			res.FeatureID = jsonString(v, "Result", "featureId")
		case "bucket":
			res.Bucket = jsonMaybeFloat(v, "Result", "bucket")
		case "key":
			res.Key = jsonString(v, "Result", "key")
		case "name":
			res.Name = jsonString(v, "Result", "name")
		case "passthrough":
			res.Passthrough = jsonBool(v, "Result", "passthrough")
		default:
			logWarn("Unknown key in JSON data", "Result", k)
		}
	}
	return &res
}
