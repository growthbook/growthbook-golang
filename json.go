package growthbook

import (
	"encoding/json"
	"fmt"
)

//  All of these functions build values of particular types from
//  representations as JSON objects. These functions are useful both
//  for testing and for user creation of GrowthBook objects from JSON
//  configuration data shared with GrowthBook SDK implementations in
//  other languages, all of which use JSON as a common configuration
//  format.

// ParseExperiment creates an Experiment value from raw JSON input.
func ParseExperiment(data []byte) *Experiment {
	dict := map[string]interface{}{}
	err := json.Unmarshal(data, &dict)
	if err != nil {
		logError(ErrJSONFailedToParse, "Experiment")
		return NewExperiment("")
	}
	return BuildExperiment(dict)
}

// BuildExperiment creates an Experiment value from a JSON object
// represented as a Go map.
func BuildExperiment(dict map[string]interface{}) *Experiment {
	exp := NewExperiment("tmp")
	gotKey := false
	for k, v := range dict {
		switch k {
		case "key":
			exp.Key = jsonString(v, "Experiment", "key")
			gotKey = true
		case "variations":
			exp = exp.WithVariations(BuildFeatureValues(v)...)
		case "ranges":
			exp = exp.WithRanges(jsonRangeArray(v, "Experiment", "ranges")...)
		case "meta":
			exp = exp.WithMeta(jsonVariationMetaArray(v, "Experiment", "meta")...)
		case "seed":
			exp = exp.WithSeed(jsonString(v, "FeatureRule", "seed"))
		case "name":
			exp = exp.WithName(jsonString(v, "FeatureRule", "name"))
		case "phase":
			exp = exp.WithPhase(jsonString(v, "FeatureRule", "phase"))
		case "weights":
			exp = exp.WithWeights(jsonFloatArray(v, "Experiment", "weights")...)
		case "active":
			exp = exp.WithActive(jsonBool(v, "Experiment", "active"))
		case "coverage":
			exp = exp.WithCoverage(jsonFloat(v, "Experiment", "coverage"))
		case "condition":
			tmp, ok := v.(map[string]interface{})
			if !ok {
				logError(ErrJSONInvalidType, "Experiment", "condition")
				continue
			}
			cond := BuildCondition(tmp)
			if cond == nil {
				logError(ErrExpJSONInvalidCondition)
			} else {
				exp = exp.WithCondition(cond)
			}
		case "namespace":
			exp = exp.WithNamespace(BuildNamespace(v))
		case "force":
			exp = exp.WithForce(jsonInt(v, "Experiment", "force"))
		case "hashAttribute":
			exp = exp.WithHashAttribute(jsonString(v, "Experiment", "hashAttribute"))
		case "hashVersion":
			exp.HashVersion = jsonInt(v, "Experiment", "hashVersion")
		default:
			logWarn(WarnJSONUnknownKey, "Experiment", k)
		}
	}
	if !gotKey {
		logWarn(WarnExpJSONKeyNotSet)
	}
	return exp
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
				logError(ErrJSONInvalidType, "Result", "hashValue")
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
		default:
			fmt.Println("OOPS: ", k)
			logWarn(WarnJSONUnknownKey, "Result", k)
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
			result.On = jsonBool(v, "FeatureResult", "on")
		case "off":
			result.Off = jsonBool(v, "FeatureResult", "off")
		case "source":
			result.Source = ParseFeatureResultSource(jsonString(v, "FeatureResult", "source"))
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
			result.ExperimentResult = BuildResult(tmp)
		default:
			logWarn(WarnJSONUnknownKey, "FeatureResult", k)
		}
	}
	return &result
}

func jsonString(v interface{}, typeName string, fieldName string) string {
	tmp, ok := v.(string)
	if ok {
		return tmp
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return ""
}

func jsonMaybeString(v interface{}, typeName string, fieldName string) *string {
	tmp, ok := v.(string)
	if ok {
		return &tmp
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return nil
}

func jsonBool(v interface{}, typeName string, fieldName string) bool {
	tmp, ok := v.(bool)
	if ok {
		return tmp
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return false
}

func jsonInt(v interface{}, typeName string, fieldName string) int {
	tmp, ok := v.(float64)
	if ok {
		return int(tmp)
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return 0
}

func jsonMaybeInt(v interface{}, typeName string, fieldName string) *int {
	tmp, ok := v.(float64)
	if ok {
		retval := int(tmp)
		return &retval
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return nil
}

func jsonFloat(v interface{}, typeName string, fieldName string) float64 {
	tmp, ok := v.(float64)
	if ok {
		return tmp
	}
	logError(ErrJSONInvalidType, typeName, fieldName)
	return 0.0
}

func jsonMaybeFloat(v interface{}, typeName string, fieldName string) *float64 {
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

func jsonRange(v interface{}, typeName string, fieldName string) *Range {
	vals := jsonFloatArray(v, typeName, fieldName)
	if vals == nil || len(vals) != 2 {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}
	return &Range{vals[0], vals[1]}
}

func jsonRangeArray(v interface{}, typeName string, fieldName string) []Range {
	vals, ok := v.([]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}
	ranges := make([]Range, len(vals))
	for i := range vals {
		tmp := jsonRange(vals[i], typeName, fieldName)
		if tmp == nil {
			return nil
		}
		ranges[i] = *tmp
	}
	return ranges
}

func jsonVariationMeta(v interface{}, typeName string, fieldName string) *VariationMeta {
	obj, ok := v.(map[string]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}

	passthrough := false
	key := ""
	name := ""
	vPassthrough, ptOk := obj["passthrough"]
	if ptOk {
		tmp, ok := vPassthrough.(bool)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		passthrough = tmp
	}
	vKey, keyOk := obj["key"]
	if keyOk {
		tmp, ok := vKey.(string)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		key = tmp
	}
	vName, nameOk := obj["name"]
	if nameOk {
		tmp, ok := vName.(string)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		name = tmp
	}

	return &VariationMeta{passthrough, key, name}
}

func jsonVariationMetaArray(v interface{}, typeName string, fieldName string) []VariationMeta {
	vals, ok := v.([]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}
	metas := make([]VariationMeta, len(vals))
	for i := range vals {
		tmp := jsonVariationMeta(vals[i], typeName, fieldName)
		if tmp == nil {
			return nil
		}
		metas[i] = *tmp
	}
	return metas
}
