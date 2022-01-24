package growthbook

import "encoding/json"

// Experiment defines a single experiment.
type Experiment struct {
	Key           string
	Variations    []FeatureValue
	Weights       []float64
	Active        bool
	Coverage      *float64
	Condition     Condition
	Namespace     *Namespace
	Force         *int
	HashAttribute *string
}

// NewExperiment creates an experiment with default settings: active,
// but all other fields empty.
func NewExperiment(key string) *Experiment {
	return &Experiment{
		Key:    key,
		Active: true,
	}
}

// WithVariations set the feature variations for an experiment.
func (exp *Experiment) WithVariations(variations ...FeatureValue) *Experiment {
	exp.Variations = variations
	return exp
}

// WithWeights set the weights for an experiment.
func (exp *Experiment) WithWeights(weights ...float64) *Experiment {
	exp.Weights = weights
	return exp
}

// WithActive sets the enabled flag for an experiment.
func (exp *Experiment) WithActive(active bool) *Experiment {
	exp.Active = active
	return exp
}

// WithCoverage sets the coverage for an experiment.
func (exp *Experiment) WithCoverage(coverage float64) *Experiment {
	exp.Coverage = &coverage
	return exp
}

// WithCondition sets the condition for an experiment.
func (exp *Experiment) WithCondition(condition Condition) *Experiment {
	exp.Condition = condition
	return exp
}

// WithNamespace sets the namespace for an experiment.
func (exp *Experiment) WithNamespace(namespace *Namespace) *Experiment {
	exp.Namespace = namespace
	return exp
}

// WithForce sets the forced value index for an experiment.
func (exp *Experiment) WithForce(force int) *Experiment {
	exp.Force = &force
	return exp
}

// WithHashAttribute sets the hash attribute for an experiment.
func (exp *Experiment) WithHashAttribute(hashAttribute string) *Experiment {
	exp.HashAttribute = &hashAttribute
	return exp
}

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
			tmp, ok := v.(string)
			if !ok {
				logError(ErrJSONInvalidType, "Experiment", "key")
				continue
			}
			exp.Key = tmp
			gotKey = true
		case "variations":
			exp = exp.WithVariations(BuildFeatureValues(v)...)
		case "weights":
			vals, ok := v.([]interface{})
			if !ok {
				logError(ErrJSONInvalidType, "Experiment", "weights")
				continue
			}
			weights := make([]float64, len(vals))
			for i := range vals {
				val, ok := vals[i].(float64)
				if !ok {
					logError(ErrJSONInvalidType, "Experiment", "weights")
					continue
				}
				weights[i] = val
			}
			exp = exp.WithWeights(weights...)
		case "active":
			tmp, ok := v.(bool)
			if !ok {
				logError(ErrJSONInvalidType, "Experiment", "key")
				continue
			}
			exp = exp.WithActive(tmp)
		case "coverage":
			tmp, ok := v.(float64)
			if !ok {
				logError(ErrJSONInvalidType, "Experiment", "coverage")
				continue
			}
			exp = exp.WithCoverage(tmp)
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
			tmp, ok := v.(float64)
			if !ok {
				logError(ErrJSONInvalidType, "Experiment", "force")
				continue
			}
			exp = exp.WithForce(int(tmp))
		case "hashAttribute":
			tmp, ok := v.(string)
			if !ok {
				logError(ErrJSONInvalidType, "Experiment", "hashAttribute")
				continue
			}
			exp = exp.WithHashAttribute(tmp)
		default:
			logWarn(WarnJSONUnknownKey, "Experiment", k)
		}
	}
	if !gotKey {
		logWarn(WarnExpJSONKeyNotSet)
	}
	return exp
}
