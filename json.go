package growthbook

import "encoding/json"

//  JSON PROCESSING HELPER FUNCTIONS
//
//  All of these functions build values of particular types from their
//  JSON representations in the test case files.

// TODO: DOCUMENTATION AND MAYBE ADD Parse... VARIANTS FOR ALL
// FUNCTIONS HERE, TO BUILD FROM RAW JSON DATA (SEE ParseFeatureMap
// AND BuildFeatureMap BELOW FOR AN EXAMPLE).

func BuildContext(dict map[string]interface{}) *Context {
	// TODO: ENSURE THAT Enabled IS GENERICALLY TRUE BY DEFAULT
	context := Context{Enabled: true}
	for k, v := range dict {
		switch k {
		case "enabled":
			context.Enabled = v.(bool)
		case "attributes":
			context.Attributes = v.(map[string]interface{})
		case "url":
			tmp := v.(string)
			context.URL = &tmp
		case "features":
			context.Features = BuildFeatureMap(v.(map[string]interface{}))
		case "forcedVariations":
			vars := map[string]int{}
			for k, vr := range v.(map[string]interface{}) {
				vars[k] = int(vr.(float64))
			}
			context.ForcedVariations = vars
		case "qaMode":
			context.QaMode = v.(bool)
		}
	}
	return &context
}

func BuildExperiment(dict map[string]interface{}) *Experiment {
	// TODO: ENSURE THAT Active IS GENERICALLY TRUE BY DEFAULT
	exp := Experiment{Active: true}
	for k, v := range dict {
		switch k {
		case "key":
			exp.Key = v.(string)
		case "variations":
			exp.Variations = v.([]interface{})
		case "weights":
			vals := v.([]interface{})
			weights := make([]float64, len(vals))
			for i := range vals {
				weights[i] = vals[i].(float64)
			}
			exp.Weights = weights
		case "active":
			exp.Active = v.(bool)
		case "coverage":
			tmp := v.(float64)
			exp.Coverage = &tmp
		case "condition":
			exp.Condition, _ = BuildCondition(v.(map[string]interface{}))
		case "namespace":
			exp.Namespace = BuildNamespace(v)
		case "force":
			tmp := int(v.(float64))
			exp.Force = &tmp
		case "hashAttribute":
			tmp := v.(string)
			exp.HashAttribute = &tmp
		}
	}
	return &exp
}

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
			rule.Variations = v.([]interface{})
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
