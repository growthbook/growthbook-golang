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
		return e.getExperimentResult(exp, -1, false, featureId, nil, false)
	}

	// 2. If context.enabled is false, return getExperimentResult(experiment)
	if !e.client.enabled {
		e.client.logger.Debug("Client disabled", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil, false)
	}

	// 3. If context.url exists
	if qsOverride, ok := getQueryStringOverride(exp.Key, e.client.url, len(exp.Variations)); ok {
		e.client.logger.Debug("Force via querystring", "id", exp.Key, "variation", qsOverride)
		return e.getExperimentResult(exp, qsOverride, false, featureId, nil, false)
	}

	// 4. Return if forced via context
	if varId, ok := e.client.forcedVariations[exp.Key]; ok {
		e.client.logger.Debug("Force via dev tools", "id", exp.Key, "variation", varId)
		return e.getExperimentResult(exp, varId, false, featureId, nil, false)
	}

	// 5. If experiment.active is set to false, return getExperimentResult(experiment)
	if !exp.getActive() {
		e.client.logger.Debug("Skip experiment because it is inactive", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil, false)
	}

	// 6. Get the user hash value and return if empty
	hashAttribute, hashValue := e.getHashAttribute(exp.HashAttribute, exp.FallbackAttribute)
	if hashValue == "" {
		e.client.logger.Debug("Skip experiment because of missing hashAttribute", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil, false)
	}

	// 6.5 If sticky bucketing is permitted, check to see if a sticky bucket value exists
	var stickyBucketFound bool = false
	var stickyBucketVariation int = -1
	var stickyBucketVersionBlocked bool = false

	if e.client.stickyBucketService != nil && !exp.DisableStickyBucketing {
		// Transform attributeValue to a map entry
		attributes := make(map[string]string)
		attributes[hashAttribute] = hashValue

		// Also add any fallback if different
		if exp.FallbackAttribute != "" && exp.FallbackAttribute != exp.HashAttribute {
			if fallbackValue, ok := e.client.attributes[exp.FallbackAttribute]; ok {
				attributes[exp.FallbackAttribute] = fallbackValue.String()
			}
		}

		// Merge with client sticky bucket attributes
		if e.client.stickyBucketAttributes != nil {
			for k, v := range e.client.stickyBucketAttributes {
				// Don't overwrite existing attributes
				if _, exists := attributes[k]; !exists {
					attributes[k] = v
				}
			}
		}

		stickyResult, err := GetStickyBucketVariation(
			exp.Key,
			exp.BucketVersion,
			exp.MinBucketVersion,
			exp.Meta,
			e.client.stickyBucketService,
			exp.HashAttribute,
			exp.FallbackAttribute,
			attributes,
			e.client.stickyBucketAssignments,
		)

		if err == nil {
			stickyBucketFound = stickyResult.Variation >= 0
			stickyBucketVariation = stickyResult.Variation
			stickyBucketVersionBlocked = stickyResult.VersionIsBlocked
		}
	}

	// Skip steps 7-8 if we found a sticky bucket or version is blocked
	if stickyBucketFound {
		e.client.logger.Debug("Found sticky bucket for experiment. Assigning sticky variation", "id", exp.Key, "variation", stickyBucketVariation)
		return e.getExperimentResult(exp, stickyBucketVariation, true, featureId, nil, true)
	}

	if stickyBucketVersionBlocked {
		return e.getExperimentResult(exp, -1, false, featureId, nil, true)
	}

	if !stickyBucketFound {
		// 7. Apply filters and namespace
		if len(exp.Filters) > 0 {
			if e.isFilteredOut(exp.Filters) {
				e.client.logger.Debug("Skip because of filters", "id", exp.Key)
				return e.getExperimentResult(exp, -1, false, featureId, nil, false)
			}
		} else if exp.Namespace != nil && !exp.Namespace.inNamespace(hashValue) {
			e.client.logger.Debug("Skip because of namespace", "id", exp.Key)
			return e.getExperimentResult(exp, -1, false, featureId, nil, false)
		}

		// 7.5. If experiment has an include property - include is deprecated property. Hence skipping this step.

		// 8 Return if any conditions are not met, return
		if !exp.Condition.Eval(e.client.attributes, e.savedGroups) {
			e.client.logger.Debug("Skip because of condition exp", "id", exp.Key)
			return e.getExperimentResult(exp, -1, false, featureId, nil, false)
		}

		// # 8.05 Exclude if parent conditions are not met
		// 8.1 If experiment.parentConditions is set (prerequisites), return if any of them evaluate to false. See the corresponding logic in
		if len(exp.ParentConditions) > 0 {
			for _, parent := range exp.ParentConditions {
				res := e.evalFeature(parent.Id)
				if res == nil {
					e.client.logger.Debug("Skip because of prerequisite fails", "id", exp.Key)
					return e.getExperimentResult(exp, -1, false, featureId, nil, false)
				}

				if res.Source == CyclicPrerequisiteResultSource {
					e.client.logger.Debug("Skip experiment because of cyclic prerequisite", "id", exp.Key)
					return e.getExperimentResult(exp, -1, false, featureId, nil, false)
				}

				evalObj := value.ObjValue{"value": value.New(res.Value)}
				evaled := parent.Condition.Eval(evalObj, e.savedGroups)
				if !evaled {
					e.client.logger.Debug("Skip because of prerequisite evaluation fails", "id", exp.Key)
					return e.getExperimentResult(exp, -1, false, featureId, nil, false)
				}
			}
		}

		//# 8.2. TODO Make sure user is in a matching group
	}

	// 8.3 TODO Apply any url targeting based on experiment.urlPatterns, return if no match

	// 9 Choose a variation - If a sticky bucket value exists, use it.
	n := hash(exp.getSeed(), hashValue, if0(exp.HashVersion, 1))
	if n == nil {
		e.client.logger.Debug("Skip because of invalid hash version", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil, false)
	}

	// 9.2 Else, calculate bucket ranges for the variations and choose one
	if !stickyBucketFound {
		ranges := exp.Ranges
		if len(exp.Ranges) == 0 {
			ranges = e.client.getBucketRanges(len(exp.Variations), exp.getCoverage(), exp.Weights)
		}
		stickyBucketVariation = chooseVariation(*n, ranges)
	}

	// # Unenroll if any prior sticky buckets are blocked by version
	if stickyBucketVersionBlocked {
		e.client.logger.Debug("Skip experiment because sticky bucket version is blocked", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil, true)
	}

	// 10. If assigned == -1, return getExperimentResult(experiment)
	if stickyBucketVariation < 0 {
		e.client.logger.Debug("Skip because of coverage", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil, false)
	}

	// 11. If experiment has a forced variation, return
	if exp.Force != nil {
		e.client.logger.Debug("Force variation", "id", exp.Key, "variation", *exp.Force)
		return e.getExperimentResult(exp, *exp.Force, false, featureId, nil, false)
	}

	// 12. If context.qaMode, return getExperimentResult(experiment)
	if e.client.qaMode {
		e.client.logger.Debug("Skip because of QA mode", "id", exp.Key)
		return e.getExperimentResult(exp, -1, false, featureId, nil, false)
	}

	// 13. Build the result object
	result := e.getExperimentResult(exp, stickyBucketVariation, true, featureId, n, stickyBucketFound)

	// 13.5 Save sticky bucket assignment if in experiment and sticky bucketing is enabled
	if e.client.stickyBucketService != nil && !exp.DisableStickyBucketing {
		// Create the sticky bucket assignment and save it
		SaveStickyBucketAssignment(
			exp.Key,
			exp.BucketVersion,
			result.VariationId,
			result.Key,
			e.client.stickyBucketService,
			hashAttribute,
			hashValue,
			e.client.stickyBucketAssignments,
		)
	}

	return result
}

func (e *evaluator) getExperimentResult(
	exp *Experiment,
	variationId int,
	hashUsed bool,
	featureId string,
	bucket *float64,
	isStickyBucketUsed bool,
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
		Key:              key,
		FeatureId:        featureId,
		InExperiment:     inExperiment,
		HashUsed:         hashUsed,
		VariationId:      variationId,
		Value:            exp.Variations[variationId],
		HashAttribute:    hashAttribute,
		HashValue:        hashValue,
		Bucket:           bucket,
		StickyBucketUsed: isStickyBucketUsed,
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
