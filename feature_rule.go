package growthbook

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
	Filters       []Filter
	Seed          string
	Name          string
	Phase         string
	// TBD:
	// Tracks?
}

// BuildFeatureRule creates an FeatureRule value from a generic JSON
// value.
func BuildFeatureRule(val interface{}) *FeatureRule {
	rule := FeatureRule{}
	dict, ok := val.(map[string]interface{})
	if !ok {
		logError(ErrJSONInvalidType, "FeatureRule")
		return &rule
	}
	for k, v := range dict {
		switch k {
		case "id":
			rule.ID = jsonString(v, "FeatureRule", "id")
		case "condition":
			condmap, ok := v.(map[string]interface{})
			if !ok {
				logError(ErrJSONInvalidType, "FeatureRule", "condition")
				continue
			}
			rule.Condition = BuildCondition(condmap)
		case "force":
			rule.Force = v
		case "variations":
			rule.Variations = BuildFeatureValues(v)
		case "weights":
			rule.Weights = jsonFloatArray(v, "FeatureRule", "weights")
		case "key":
			rule.Key = jsonString(v, "FeatureRule", "key")
		case "hashAttribute":
			rule.HashAttribute = jsonString(v, "FeatureRule", "hashAttribute")
		case "hashVersion":
			rule.HashVersion = jsonInt(v, "FeatureRule", "hashVersion")
		case "range":
			rule.Range = jsonRange(v, "FeatureRule", "range")
		case "coverage":
			rule.Coverage = jsonMaybeFloat(v, "FeatureRule", "coverage")
		case "namespace":
			rule.Namespace = BuildNamespace(v)
		case "ranges":
			rule.Ranges = jsonRangeArray(v, "FeatureRule", "ranges")
		case "meta":
			rule.Meta = jsonVariationMetaArray(v, "Experiment", "meta")
		case "filters":
			rule.Filters = jsonFilterArray(v, "Experiment", "filters")
		case "seed":
			rule.Seed = jsonString(v, "FeatureRule", "seed")
		case "name":
			rule.Name = jsonString(v, "FeatureRule", "name")
		case "phase":
			rule.Phase = jsonString(v, "FeatureRule", "phase")
		default:
			logWarn(WarnJSONUnknownKey, "FeatureRule", k)
		}
	}
	return &rule
}
