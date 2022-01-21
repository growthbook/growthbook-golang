package growthbook

import "encoding/json"

//  JSON PROCESSING HELPER FUNCTIONS
//
//  All of these functions build values of particular types from
//  representations as JSON objects. These functions are useful both
//  for testing and for user creation of GrowthBook objects from JSON
//  configuration data shared with GrowthBook SDK implementations in
//  other languages, all of which use JSON as a common configuration
//  format.

// TODO: DOCUMENTATION AND MAYBE ADD Parse... VARIANTS FOR ALL
// FUNCTIONS HERE, TO BUILD FROM RAW JSON DATA (SEE ParseFeatureMap
// AND BuildFeatureMap BELOW FOR AN EXAMPLE).

// INPUT DATA TYPES (USEFUL TO HAVE PUBLICLY VISIBLE JSON CONVERSION
// FUNCTIONS):
//
//  - Context => Attributes, FeatureMap, ForcedVariationsMap
//  - Attributes
//  - FeatureMap => Feature
//  - ForcedVariationsMap
//  - Experiment => Condition, Namespace
//  - Feature => FeatureRule
//  - Condition
//  - Namespace
//  - FeatureRule => Condition, Namespace

// OUTPUT DATA TYPES (JSON CONVERSION USED ONLY FOR TESTING):
//
//  - ExperimentResult
//  - FeatureResult

// BuildExperimentResult creates an ExperimentResult value from a JSON
// object represented as a Go map.
func BuildExperimentResult(dict map[string]interface{}) *ExperimentResult {
	// TODO: ENSURE THAT Active IS GENERICALLY TRUE BY DEFAULT
	res := ExperimentResult{}
	for k, v := range dict {
		switch k {
		case "inExperiment":
			res.InExperiment = v.(bool)
		case "variationId":
			res.VariationID = int(v.(float64))
		case "value":
			res.Value = v
		case "hashAttribute":
			res.HashAttribute = v.(string)
		case "hashValue":
			res.HashValue = v.(string)
		}
	}
	return &res
}

func BuildFeatureValues(val interface{}) []FeatureValue {
	vals := val.([]interface{})
	result := make([]FeatureValue, len(vals))
	for i, v := range vals {
		result[i] = v.(FeatureValue)
	}
	return result
}

func ParseFeatureMap(data []byte) (FeatureMap, error) {
	dict := map[string]interface{}{}
	err := json.Unmarshal(data, &dict)
	if err != nil {
		return nil, err
	}
	return BuildFeatureMap(dict), nil
}

func BuildFeatureMap(dict map[string]interface{}) FeatureMap {
	fmap := FeatureMap{}
	for k, v := range dict {
		fmap[k] = BuildFeature(v)
	}
	return fmap
}

func BuildFeature(val interface{}) *Feature {
	feature := Feature{}
	dict, ok := val.(map[string]interface{})
	if !ok {
		return &feature
	}
	defaultValue, ok := dict["defaultValue"]
	if ok {
		feature.DefaultValue = defaultValue
	}
	rules, ok := dict["rules"]
	if ok {
		rulesArray := rules.([]interface{})
		feature.Rules = make([]*FeatureRule, len(rulesArray))
		for i := range rulesArray {
			feature.Rules[i] = BuildFeatureRule(rulesArray[i])
		}
	}
	return &feature
}

func BuildFeatureRule(val interface{}) *FeatureRule {
	rule := FeatureRule{}
	dict, ok := val.(map[string]interface{})
	if !ok {
		return &rule
	}
	for k, v := range dict {
		switch k {
		case "condition":
			rule.Condition, _ = BuildCondition(v.(map[string]interface{}))
		case "coverage":
			tmp := v.(float64)
			rule.Coverage = &tmp
		case "force":
			rule.Force = v
		case "variations":
			rule.Variations = BuildFeatureValues(v)
		case "key":
			tmp := v.(string)
			rule.TrackingKey = &tmp
		case "weights":
			vals := v.([]interface{})
			weights := make([]float64, len(vals))
			for i := range vals {
				weights[i] = vals[i].(float64)
			}
			rule.Weights = weights
		case "namespace":
			rule.Namespace = BuildNamespace(v)
		case "hashAttribute":
			tmp := v.(string)
			rule.HashAttribute = &tmp
		}
	}
	return &rule
}

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

func BuildFeatureResult(dict map[string]interface{}) *FeatureResult {
	result := FeatureResult{}
	result.Value = dict["value"]
	result.On = dict["on"].(bool)
	result.Off = dict["off"].(bool)
	result.Source = ParseFeatureResultSource(dict["source"].(string))
	experimentDict, ok := dict["experiment"].(map[string]interface{})
	if ok {
		result.Experiment = BuildExperiment(experimentDict)
	}
	experimentResultDict, ok := dict["experimentResult"].(map[string]interface{})
	if ok {
		result.ExperimentResult = BuildExperimentResult(experimentResultDict)
	}
	return &result
}
