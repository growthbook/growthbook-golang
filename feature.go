package growthbook

import "encoding/json"

// FeatureValue is a wrapper around an arbitrary type representing the
// value of a feature. Features can return any kinds of values, so
// this is an alias for interface{}.
type FeatureValue interface{}

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
		logError("Failed parsing JSON input", "Feature")
		return nil
	}
	return BuildFeature(dict)
}

// BuildFeature creates a Feature value from a generic JSON value.
func BuildFeature(val interface{}) *Feature {
	feature := Feature{}
	dict, ok := val.(map[string]interface{})
	if !ok {
		logError("Invalid JSON data type", "Feature")
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
			logError("Invalid JSON data type", "Feature")
			return &feature
		}
		feature.Rules = make([]*FeatureRule, len(rulesArray))
		for i := range rulesArray {
			feature.Rules[i] = BuildFeatureRule(rulesArray[i])
		}
	}
	return &feature
}

// BuildFeatureValues creates a FeatureValue array from a generic JSON
// value.
func BuildFeatureValues(val interface{}) []FeatureValue {
	vals, ok := val.([]interface{})
	if !ok {
		logError("Invalid JSON data type", "FeatureValue")
		return nil
	}
	result := make([]FeatureValue, len(vals))
	for i, v := range vals {
		tmp, ok := v.(FeatureValue)
		if !ok {
			logError("Invalid JSON data type", "FeatureValue")
			return nil
		}
		result[i] = tmp
	}
	return result
}
