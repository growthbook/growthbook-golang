package growthbook

import (
	"github.com/growthbook/growthbook-golang/internal/value"
)

type evaluator struct {
	features   FeatureMap
	attributes value.ObjValue
	evaluated  stack[string]
}

func (e *evaluator) evalFeature(key string) *FeatureResult {
	if e.evaluated.has(key) {
		return getFeatureResult(nil, CyclicPrerequisiteResultSource, "", nil, nil)
	}
	e.evaluated.push(key)
	defer e.evaluated.pop()

	feature := e.features[key]
	if feature == nil {
		return getFeatureResult(nil, UnknownFeatureResultSource, "", nil, nil)
	}

	for _, rule := range feature.Rules {
		res := e.evalRule(&rule)
		if res != nil {
			return res
		}
	}

	return nil
}

func (e *evaluator) evalRule(rule *FeatureRule) *FeatureResult {
	if len(rule.ParentConditions) > 0 {
		for _, parent := range rule.ParentConditions {
			res := e.evalFeature(parent.Id)
			if res == nil {
				return nil
			}

			if res.Source == CyclicPrerequisiteResultSource {
				return res
			}

			evalObj := value.ObjValue{"value": value.New(res.Value)}
			evaled := parent.Condition.Eval(evalObj, nil)
			if !evaled {
				if parent.Gate {
					return getFeatureResult(nil, PrerequisiteResultSource, "", nil, nil)
				}
				return nil
			}
		}
	}

	if e.isFilteredOut(rule.Filters) {
		return nil
	}

	if rule.Force != nil {
		if !rule.Condition.Eval(e.attributes, nil) {
			return nil
		}

		if !e.isIncludedInRollout(rule) {
			return nil
		}

		return getFeatureResult(rule.Force, ForceResultSource, rule.Id, nil, nil)
	}

	return nil
}

func (e *evaluator) isIncludedInRollout(rule *FeatureRule) bool {
	return true
}

func (e *evaluator) isFilteredOut(filters []Filter) bool {
	for _, filter := range filters {
		_, hashValue := e.getHashAttribute(filter.Attribute, "")
		if hashValue == value.Null() {
			return true
		}

		hash := hash(filter.Seed, hashValue, if0(filter.HashVersion, 2))
		if hash == nil {
			return true
		}
		if chooseVariation(*hash, filter.Ranges) == -1 {
			return true
		}
	}
	return false
}

func (e *evaluator) getHashAttribute(key string, fallback string) (string, value.Value) {
	if key == "" {
		key = "id"
	}

	if hashValue, ok := e.attributes[key]; ok {
		return key, hashValue
	}

	if hashValue, ok := e.attributes[fallback]; ok {
		return fallback, hashValue
	}

	return key, value.Null()
}
