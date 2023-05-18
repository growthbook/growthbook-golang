package growthbook

import (
	"encoding/json"
)

// Experiment defines a single experiment.
type Experiment struct {
	Key        string
	Variations []FeatureValue
	Ranges     []Range
	Meta       []VariationMeta
	Filters    []Filter
	Seed       string
	Name       string
	Phase      string
	// URLPatterns
	Weights   []float64
	Condition Condition
	Coverage  *float64
	// Include?
	Namespace     *Namespace
	Force         *int
	Active        bool
	HashAttribute string
	HashVersion   int
	// Status
	// URL
	// Groups
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

// WithRanges set the ranges for an experiment.
func (exp *Experiment) WithRanges(ranges ...Range) *Experiment {
	exp.Ranges = ranges
	return exp
}

// WithMeta sets the meta information for an experiment.
func (exp *Experiment) WithMeta(meta ...VariationMeta) *Experiment {
	exp.Meta = meta
	return exp
}

// WithFilters sets the filters for an experiment.
func (exp *Experiment) WithFilters(filters ...Filter) *Experiment {
	exp.Filters = filters
	return exp
}

// WithWeights set the weights for an experiment.
func (exp *Experiment) WithWeights(weights ...float64) *Experiment {
	exp.Weights = weights
	return exp
}

// WithSeed sets the hash seed for an experiment.
func (exp *Experiment) WithSeed(seed string) *Experiment {
	exp.Seed = seed
	return exp
}

// WithName sets the name for an experiment.
func (exp *Experiment) WithName(name string) *Experiment {
	exp.Name = name
	return exp
}

// WithPhase sets the phase for an experiment.
func (exp *Experiment) WithPhase(phase string) *Experiment {
	exp.Phase = phase
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
	exp.HashAttribute = hashAttribute
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
			exp.Key = jsonString(v, "Experiment", "key")
			gotKey = true
		case "variations":
			exp = exp.WithVariations(BuildFeatureValues(v)...)
		case "ranges":
			exp = exp.WithRanges(jsonRangeArray(v, "Experiment", "ranges")...)
		case "meta":
			exp = exp.WithMeta(jsonVariationMetaArray(v, "Experiment", "meta")...)
		case "filters":
			exp = exp.WithFilters(jsonFilterArray(v, "Experiment", "filters")...)
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
