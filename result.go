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
			variationID, ok := jsonInt(v, "Result", "variationId")
			if !ok {
				return nil
			}
			res.VariationID = variationID
		case "inExperiment":
			inExperiment, ok := jsonBool(v, "Result", "inExperiment")
			if !ok {
				return nil
			}
			res.InExperiment = inExperiment
		case "hashUsed":
			hashUsed, ok := jsonBool(v, "Result", "hashUsed")
			if !ok {
				return nil
			}
			res.HashUsed = hashUsed
		case "hashAttribute":
			hashAttribute, ok := jsonString(v, "Result", "hashAttribute")
			if !ok {
				return nil
			}
			res.HashAttribute = hashAttribute
		case "hashValue":
			tmp, ok := convertHashValue(v)
			if !ok {
				logError("Invalid JSON data type", "Result", "hashValue")
				return nil
			}
			res.HashValue = tmp
		case "featureId":
			featureID, ok := jsonString(v, "Result", "featureId")
			if !ok {
				return nil
			}
			res.FeatureID = featureID
		case "bucket":
			bucket, ok := jsonMaybeFloat(v, "Result", "bucket")
			if !ok {
				return nil
			}
			res.Bucket = bucket
		case "key":
			key, ok := jsonString(v, "Result", "key")
			if !ok {
				return nil
			}
			res.Key = key
		case "name":
			name, ok := jsonString(v, "Result", "name")
			if !ok {
				return nil
			}
			res.Name = name
		case "passthrough":
			passthrough, ok := jsonBool(v, "Result", "passthrough")
			if !ok {
				return nil
			}
			res.Passthrough = passthrough
		default:
			logWarn("Unknown key in JSON data", "Result", k)
		}
	}
	return &res
}
