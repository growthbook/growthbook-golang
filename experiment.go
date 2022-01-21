package growthbook

import "encoding/json"

// Experiment defines a single experiment.
type Experiment struct {
	Key           string         // Required: set in NewExperiment
	Variations    []FeatureValue // Optional: (OK: array)
	Weights       []float64      // Optional: (OK: array)
	Active        bool           // Required: set in NewExperiment
	Coverage      *float64       // Optional: (OK: pointer)
	Condition     Condition      // Optional: (OK: interface)
	Namespace     *Namespace     // Optional: (OK: pointer)
	Force         *int           // Optional: (OK: pointer)
	HashAttribute *string        // Optional: (OK: pointer)
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
		logError(ErrExpJSONFailedToParse)
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
			exp.Key = v.(string)
			gotKey = true
		case "variations":
			exp = exp.WithVariations(BuildFeatureValues(v)...)
		case "weights":
			vals := v.([]interface{})
			weights := make([]float64, len(vals))
			for i := range vals {
				weights[i] = vals[i].(float64)
			}
			exp = exp.WithWeights(weights...)
		case "active":
			exp = exp.WithActive(v.(bool))
		case "coverage":
			exp = exp.WithCoverage(v.(float64))
		case "condition":
			cond, err := BuildCondition(v.(map[string]interface{}))
			if err != nil {
				logError(ErrExpJSONInvalidCondition)
			} else {
				exp = exp.WithCondition(cond)
			}
		case "namespace":
			exp = exp.WithNamespace(BuildNamespace(v))
		case "force":
			exp = exp.WithForce(int(v.(float64)))
		case "hashAttribute":
			exp = exp.WithHashAttribute(v.(string))
		}
	}
	if !gotKey {
		logWarn(WarnExpJSONKeyNotSet)
	}
	return exp
}
