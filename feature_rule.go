package growthbook

// FeatureRule overrides the default value of a Feature.
type FeatureRule struct {
	ID            string          `json:",omitempty"`
	Condition     Condition       `json:"condition,omitempty"`
	Force         FeatureValue    `json:"force,omitempty"`
	Variations    []FeatureValue  `json:"variations,omitempty"`
	Weights       []float64       `json:"weights,omitempty"`
	Key           string          `json:"key,omitempty"`
	HashAttribute string          `json:"hashAttribute,omitempty"`
	HashVersion   int             `json:"hashVersion,omitempty"`
	Range         *Range          `json:"range,omitempty"`
	Coverage      *float64        `json:"coverage,omitempty"`
	Namespace     *Namespace      `json:"namespace,omitempty"`
	Ranges        []Range         `json:"ranges,omitempty"`
	Meta          []VariationMeta `json:"meta,omitempty"`
	Filters       []Filter        `json:"filters,omitempty"`
	Seed          string          `json:"seed,omitempty"`
	Name          string          `json:"name,omitempty"`
	Phase         string          `json:"phase,omitempty"`
}

// BuildFeatureRule creates an FeatureRule value from a generic JSON
// value.
func BuildFeatureRule(val interface{}) *FeatureRule {
	rule := FeatureRule{}
	dict, ok := val.(map[string]interface{})
	if !ok {
		logError("Invalid JSON data type", "FeatureRule")
		return nil
	}
	for k, v := range dict {
		switch k {
		case "id":
			id, ok := jsonString(v, "FeatureRule", "id")
			if !ok {
				return nil
			}
			rule.ID = id
		case "condition":
			condmap, ok := v.(map[string]interface{})
			if !ok {
				logError("Invalid JSON data type", "FeatureRule", "condition")
				return nil
			}
			condition := BuildCondition(condmap)
			if condition == nil {
				return nil
			}
			rule.Condition = condition
		case "force":
			rule.Force = v
		case "variations":
			variations := BuildFeatureValues(v)
			if variations == nil {
				return nil
			}
			rule.Variations = variations
		case "weights":
			weights, ok := jsonFloatArray(v, "FeatureRule", "weights")
			if !ok {
				return nil
			}
			rule.Weights = weights
		case "key":
			key, ok := jsonString(v, "FeatureRule", "key")
			if !ok {
				return nil
			}
			rule.Key = key
		case "hashAttribute":
			hashAttribute, ok := jsonString(v, "FeatureRule", "hashAttribute")
			if !ok {
				return nil
			}
			rule.HashAttribute = hashAttribute
		case "hashVersion":
			hashVersion, ok := jsonInt(v, "FeatureRule", "hashVersion")
			if !ok {
				return nil
			}
			rule.HashVersion = hashVersion
		case "range":
			rng, ok := jsonRange(v, "FeatureRule", "range")
			if !ok {
				return nil
			}
			rule.Range = rng
		case "coverage":
			coverage, ok := jsonMaybeFloat(v, "FeatureRule", "coverage")
			if !ok {
				return nil
			}
			rule.Coverage = coverage
		case "namespace":
			namespace := BuildNamespace(v)
			if namespace == nil {
				return nil
			}
			rule.Namespace = namespace
		case "ranges":
			ranges, ok := jsonRangeArray(v, "FeatureRule", "ranges")
			if !ok {
				return nil
			}
			rule.Ranges = ranges
		case "meta":
			meta, ok := jsonVariationMetaArray(v, "Experiment", "meta")
			if !ok {
				return nil
			}
			rule.Meta = meta
		case "filters":
			filters, ok := jsonFilterArray(v, "Experiment", "filters")
			if !ok {
				return nil
			}
			rule.Filters = filters
		case "seed":
			seed, ok := jsonString(v, "FeatureRule", "seed")
			if !ok {
				return nil
			}
			rule.Seed = seed
		case "name":
			name, ok := jsonString(v, "FeatureRule", "name")
			if !ok {
				return nil
			}
			rule.Name = name
		case "phase":
			phase, ok := jsonString(v, "FeatureRule", "phase")
			if !ok {
				return nil
			}
			rule.Phase = phase
		default:
			logWarn("Unknown key in JSON data", "FeatureRule", k)
		}
	}
	return &rule
}
