package growthbook

import "encoding/json"

// Feature has a default value plus rules than can override the
// default.
type Feature struct {
	DefaultValue FeatureValue
	Rules        []*FeatureRule
}

// ParseFeature creates a single Feature value from raw JSON input.
func ParseFeature(data []byte) *Feature {
	dict := map[string]interface{}{}
	err := json.Unmarshal(data, &dict)
	if err != nil {
		logError(ErrJSONFailedToParse, "Feature")
		return nil
	}
	return BuildFeature(dict)
}

// BuildFeature creates a Feature value from a generic JSON value.
func BuildFeature(val interface{}) *Feature {
	feature := Feature{}
	dict, ok := val.(map[string]interface{})
	if !ok {
		logError(ErrJSONInvalidType, "Feature")
		return &feature
	}
	defaultValue, ok := dict["defaultValue"]
	if ok {
		feature.DefaultValue = defaultValue
	}
	rules, ok := dict["rules"]
	if ok {
		rulesArray, ok := rules.([]interface{})
		if !ok {
			logError(ErrJSONInvalidType, "Feature")
			return &feature
		}
		feature.Rules = make([]*FeatureRule, len(rulesArray))
		for i := range rulesArray {
			feature.Rules[i] = BuildFeatureRule(rulesArray[i])
		}
	}
	return &feature
}
