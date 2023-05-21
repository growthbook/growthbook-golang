package growthbook

import (
	"encoding/json"
)

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

// ParseFeatureMap creates a FeatureMap value from raw JSON input.
func ParseFeatureMap(data []byte) FeatureMap {
	dict := map[string]interface{}{}
	err := json.Unmarshal(data, &dict)
	if err != nil {
		logError("Failed parsing JSON input", "FeatureMap")
		return nil
	}
	return BuildFeatureMap(dict)
}

// BuildFeatureMap creates a FeatureMap value from a JSON object
// represented as a Go map.
func BuildFeatureMap(dict map[string]interface{}) FeatureMap {
	fmap := FeatureMap{}
	for k, v := range dict {
		fmap[k] = BuildFeature(v)
	}
	return fmap
}
