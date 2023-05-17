package growthbook

import (
	"encoding/json"
)

//  All of these functions build values of particular types from
//  representations as JSON objects. These functions are useful both
//  for testing and for user creation of GrowthBook objects from JSON
//  configuration data shared with GrowthBook SDK implementations in
//  other languages, all of which use JSON as a common configuration
//  format.

// BuildExperimentResult creates an ExperimentResult value from a JSON
// object represented as a Go map.
func BuildExperimentResult(dict map[string]interface{}) *ExperimentResult {
	res := ExperimentResult{}
	for k, v := range dict {
		switch k {
		case "value":
			res.Value = v
		case "variationId":
			tmp, ok := v.(float64)
			if !ok {
				logError(ErrJSONInvalidType, "ExperimentResult", "variationId")
				continue
			}
			res.VariationID = int(tmp)
		case "inExperiment":
			tmp, ok := v.(bool)
			if !ok {
				logError(ErrJSONInvalidType, "ExperimentResult", "inExperiment")
				continue
			}
			res.InExperiment = tmp
		case "hashUsed":
			tmp, ok := v.(bool)
			if !ok {
				logError(ErrJSONInvalidType, "ExperimentResult", "hashUsed")
				continue
			}
			res.HashUsed = tmp
		case "hashAttribute":
			tmp, ok := v.(string)
			if !ok {
				logError(ErrJSONInvalidType, "ExperimentResult", "hashAttribute")
				continue
			}
			res.HashAttribute = tmp
		case "hashValue":
			tmp, ok := convertHashValue(v)
			if !ok {
				logError(ErrJSONInvalidType, "ExperimentResult", "hashValue")
				continue
			}
			res.HashValue = tmp
		case "featureId":
			tmp, ok := v.(string)
			if !ok {
				logError(ErrJSONInvalidType, "ExperimentResult", "featureId")
				continue
			}
			res.FeatureID = &tmp
		default:
			logWarn(WarnJSONUnknownKey, "ExperimentResult", k)
		}
	}
	return &res
}

// BuildFeatureValues creates a FeatureValue array from a generic JSON
// value.
func BuildFeatureValues(val interface{}) []FeatureValue {
	vals, ok := val.([]interface{})
	if !ok {
		logError(ErrJSONInvalidType, "FeatureValue")
		return nil
	}
	result := make([]FeatureValue, len(vals))
	for i, v := range vals {
		tmp, ok := v.(FeatureValue)
		if !ok {
			logError(ErrJSONInvalidType, "FeatureValue")
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
		logError(ErrJSONFailedToParse, "FeatureMap")
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

// BuildFeatureRule creates an FeatureRule value from a generic JSON
// value.
func BuildFeatureRule(val interface{}) *FeatureRule {
	rule := FeatureRule{}
	dict, ok := val.(map[string]interface{})
	if !ok {
		logError(ErrJSONInvalidType, "FeatureRule")
		return &rule
	}
KeyLoop:
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
			vals := jsonFloatArray(v, "FeatureRule", "range")
			if vals != nil {
				if len(vals) != 2 {
					logError(ErrJSONInvalidType, "FeatureRule", "ranges")
					continue
				}
				rule.Range = &Range{vals[0], vals[1]}
			}
		case "coverage":
			rule.Coverage = jsonFloat(v, "FeatureRule", "coverage")
		case "namespace":
			rule.Namespace = BuildNamespace(v)
		case "ranges":
			vals, ok := v.([]interface{})
			if !ok {
				logError(ErrJSONInvalidType, "FeatureRule", "ranges")
				continue
			}
			ranges := make([]Range, len(vals))
			for i := range vals {
				tmp := jsonFloatArray(vals[i], "FeatureRule", "ranges")
				if tmp == nil || len(tmp) != 2 {
					logError(ErrJSONInvalidType, "FeatureRule", "ranges")
					continue KeyLoop
				}
				ranges[i] = Range{tmp[0], tmp[1]}
			}
			rule.Ranges = ranges
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

func jsonString(v interface{}, typeName string, fieldName string) *string {
	tmp, ok := v.(string)
	if ok {
		return &tmp
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return nil
}

func jsonInt(v interface{}, typeName string, fieldName string) *int {
	tmp, ok := v.(float64)
	if ok {
		retval := int(tmp)
		return &retval
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return nil
}

func jsonFloat(v interface{}, typeName string, fieldName string) *float64 {
	tmp, ok := v.(float64)
	if ok {
		return &tmp
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return nil
}

func jsonFloatArray(v interface{}, typeName string, fieldName string) []float64 {
	vals, ok := v.([]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}
	fvals := make([]float64, len(vals))
	for i := range vals {
		tmp, ok := vals[i].(float64)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		fvals[i] = tmp
	}
	return fvals
}

// ParseNamespace creates a Namespace value from raw JSON input.
func ParseNamespace(data []byte) *Namespace {
	array := []interface{}{}
	err := json.Unmarshal(data, &array)
	if err != nil {
		logError(ErrJSONFailedToParse, "Namespace")
		return nil
	}
	return BuildNamespace(array)
}

// BuildNamespace creates a Namespace value from a generic JSON value.
func BuildNamespace(val interface{}) *Namespace {
	array, ok := val.([]interface{})
	if !ok || len(array) != 3 {
		return nil
	}
	id, ok1 := array[0].(string)
	start, ok2 := array[1].(float64)
	end, ok3 := array[2].(float64)
	if !ok1 || !ok2 || !ok3 {
		return nil
	}
	return &Namespace{id, start, end}
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
			tmp, ok := v.(bool)
			if !ok {
				logError(ErrJSONInvalidType, "FeatureResult", "on")
				continue
			}
			result.On = tmp
		case "off":
			tmp, ok := v.(bool)
			if !ok {
				logError(ErrJSONInvalidType, "FeatureResult", "off")
				continue
			}
			result.Off = tmp
		case "source":
			tmp, ok := v.(string)
			if !ok {
				logError(ErrJSONInvalidType, "FeatureResult", "source")
				continue
			}
			result.Source = ParseFeatureResultSource(tmp)
		case "experiment":
			tmp, ok := v.(map[string]interface{})
			if !ok {
				logError(ErrJSONInvalidType, "FeatureResult", "experiment")
				continue
			}
			result.Experiment = BuildExperiment(tmp)
		case "experimentResult":
			tmp, ok := v.(map[string]interface{})
			if !ok {
				logError(ErrJSONInvalidType, "FeatureResult", "experimentResult")
				continue
			}
			result.ExperimentResult = BuildExperimentResult(tmp)
		default:
			logWarn(WarnJSONUnknownKey, "FeatureResult", k)
		}
	}
	return &result
}
