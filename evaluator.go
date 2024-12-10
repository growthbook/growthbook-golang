package growthbook

import (
	"fmt"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
)

type evaluator struct {
	features    FeatureMap
	savedGroups condition.SavedGroups
	evaluated   stack[string]
	client      *Client
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
		res := e.evalRule(key, &rule)
		if res != nil {
			return res
		}
	}

	return getFeatureResult(feature.DefaultValue, DefaultValueResultSource, "", nil, nil)
}

func (e *evaluator) runExperiment(exp *Experiment, featureId string) *ExperimentResult {

	// 1. If experiment.variations has fewer than 2 variations, return getExperimentResult(experiment)
	if len(exp.Variations) < 2 {
		e.client.logger.Debug("Invalid experiment", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}

	// 2. If context.enabled is false, return getExperimentResult(experiment)
	if !e.client.enabled {
		e.client.logger.Debug("Client disabled", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}

	// 3. If context.url exists
	if qsOverride, ok := getQueryStringOverride(exp.Key, e.client.url, len(exp.Variations)); ok {
		e.client.logger.Debug("Force via querystring", "id", exp.Key, "variation", qsOverride)
		return e.getExperimentResult(exp, qsOverride, false, featureId, nil)
	}

	// 4. Return if forced via context
	if varId, ok := e.client.forcedVariations[exp.Key]; ok {
		e.client.logger.Debug("Force via dev tools", "id", exp.Key, "variation", varId)
		return e.getExperimentResult(exp, varId, false, featureId, nil)
	}

	// 5. If experiment.active is set to false, return getExperimentResult(experiment)
	if !exp.getActive() {
		e.client.logger.Debug("Skip because inactive", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}

	// 6. Get the user hash value and return if empty
	_, hashValue := e.getHashAttribute(exp.HashAttribute, exp.FallbackAttribute)
	if hashValue == "" {
		e.client.logger.Debug("Skip because of missing hashAttribute", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}

	// 6.5 TODO If sticky bucketing is permitted, check to see if a sticky bucket value exists. If so, skip steps 7-8.

	// 7. Apply filters and namespace

	if len(exp.Filters) > 0 {
		if e.isFilteredOut(exp.Filters) {
			e.client.logger.Debug("Skip because of filters", "id", exp.Key)
			return e.getExperimentResult(exp, -1, false, featureId, nil)
		}
	} else if exp.Namespace != nil && !exp.Namespace.inNamespace(hashValue) {
		e.client.logger.Debug("Skip because of namespace", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}

	// 8 Return if any conditions are not met, return
	if !exp.Condition.Eval(e.client.attributes, e.savedGroups) {
		e.client.logger.Debug("Skip because of condition exp", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}

	// 8.2 If experiment.parentConditions is set (prerequisites), return if any of them evaluate to false. See the corresponding logic in
	if len(exp.ParentConditions) > 0 {
		for _, parent := range exp.ParentConditions {
			res := e.evalFeature(parent.Id)
			if res == nil {
				e.client.logger.Debug("Skip because of prerequisite fails", "id", exp.Key)
				return e.getExperimentResult(exp, -1, false, featureId, nil)
			}

			if res.Source == CyclicPrerequisiteResultSource {
				return e.getExperimentResult(exp, -1, false, featureId, nil)
			}

			evalObj := value.ObjValue{"value": value.New(res.Value)}
			evaled := parent.Condition.Eval(evalObj, e.savedGroups)
			if !evaled {
				e.client.logger.Debug("Skip because of prerequisite evaluation fails", "id", exp.Key)
				return e.getExperimentResult(exp, -1, false, featureId, nil)
			}
		}
	}

	// 8.3 TODO Apply any url targeting based on experiment.urlPatterns, return if no match

	// 9 Choose a variation
	// 9.1 TODO If a sticky bucket value exists, use it.

	// 9.2 Else, calculate bucket ranges for the variations and choose one
	ranges := exp.Ranges
	if len(exp.Ranges) == 0 {
		ranges = e.client.getBucketRanges(len(exp.Variations), exp.getCoverage(), exp.Weights)
	}

	n := hash(exp.getSeed(), hashValue, if0(exp.HashVersion, 1))
	if n == nil {
		e.client.logger.Debug("Skip because of invalid hash version", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}
	assigned := chooseVariation(*n, ranges)

	// 10. If assigned == -1, return getExperimentResult(experiment)
	if assigned < 0 {
		e.client.logger.Debug("Skip because of coverage", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}

	// 11. If experiment has a forced variation, return
	if exp.Force != nil {
		e.client.logger.Debug("Force variation", "id", exp.Key, "variation", *exp.Force)
		return e.getExperimentResult(exp, *exp.Force, false, featureId, nil)
	}

	// 12. If context.qaMode, return getExperimentResult(experiment)
	if e.client.qaMode {
		e.client.logger.Debug("Skip because of QA mode", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil)
	}

	// 13. Build the result object
	return e.getExperimentResult(exp, assigned, true, featureId, n)
}

func (e *evaluator) getExperimentResult(
	exp *Experiment,
	variationId int,
	hashUsed bool,
	featureId string,
	bucket *float64,
) *ExperimentResult {
	inExperiment := true

	if variationId < 0 || variationId >= len(exp.Variations) {
		variationId = 0
		inExperiment = false
	}

	hashAttribute, hashValue := e.getHashAttribute(exp.HashAttribute, "")

	var meta *VariationMeta
	if variationId > 0 && variationId < len(exp.Meta) {
		meta = &exp.Meta[variationId]
	}

	key := fmt.Sprint(variationId)
	if meta != nil && meta.Key != "" {
		key = meta.Key
	}

	res := ExperimentResult{
		Key:           key,
		FeatureId:     featureId,
		InExperiment:  inExperiment,
		HashUsed:      hashUsed,
		VariationId:   variationId,
		Value:         exp.Variations[variationId],
		HashAttribute: hashAttribute,
		HashValue:     hashValue,
		Bucket:        bucket,
	}

	if meta != nil {
		res.Name = meta.Name
		res.Passthrough = meta.Passthrough
	}

	return &res
}

func (e *evaluator) evalRule(featureId string, rule *FeatureRule) *FeatureResult {
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
			evaled := parent.Condition.Eval(evalObj, e.savedGroups)
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
		if !rule.Condition.Eval(e.client.attributes, e.savedGroups) {
			return nil
		}

		if !e.isIncludedInRollout(featureId, rule) {
			return nil
		}

		return getFeatureResult(rule.Force, ForceResultSource, rule.Id, nil, nil)
	}

	if len(rule.Variations) == 0 {
		return nil
	}

	exp := experimentFromFeatureRule(featureId, rule)
	res := e.runExperiment(exp, featureId)
	if !res.InExperiment || res.Passthrough {
		return nil
	}

	return getFeatureResult(res.Value, ExperimentResultSource, rule.Id, exp, res)
}

func (e *evaluator) isIncludedInRollout(featureId string, rule *FeatureRule) bool {
	if rule == nil {
		return true
	}

	if rule.Coverage == nil && rule.Range == nil {
		return true
	}

	if rule.Range == nil && *rule.Coverage == 0.0 {
		return false
	}

	_, hashValue := e.getHashAttribute(rule.HashAttribute, "")
	if hashValue == "" {
		return false
	}

	seed := rule.Seed
	if seed == "" {
		seed = featureId
	}
	n := hash(seed, hashValue, if0(rule.HashVersion, 1))
	if n == nil {
		return false
	}

	if rule.Range != nil {
		return rule.Range.InRange(*n)
	}

	if rule.Coverage != nil {
		return *n <= *rule.Coverage
	}

	return true
}

func (e *evaluator) isFilteredOut(filters []Filter) bool {
	for _, filter := range filters {
		_, hashValue := e.getHashAttribute(filter.Attribute, "")
		if hashValue == "" {
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

func (e *evaluator) getHashAttribute(key string, fallback string) (string, string) {
	if key == "" {
		key = "id"
	}

	hashValue, ok := e.client.attributes[key]
	if ok && !value.IsNull(hashValue) {
		return key, hashValue.String()
	}

	hashValue, ok = e.client.attributes[fallback]
	if ok && !value.IsNull(hashValue) {
		return fallback, hashValue.String()
	}

	return key, ""
}
