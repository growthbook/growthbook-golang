package growthbook

import (
	"encoding/json"
	"regexp"
)

type ExperimentStatus string

const (
	DraftStatus   ExperimentStatus = "draft"
	RunningStatus ExperimentStatus = "running"
	StoppedStatus ExperimentStatus = "stopped"
)

// Experiment defines a single experiment.
type Experiment struct {
	Key           string
	Variations    []FeatureValue
	Ranges        []Range
	Meta          []VariationMeta
	Filters       []Filter
	Seed          string
	Name          string
	Phase         string
	URLPatterns   []URLTarget
	Weights       []float64
	Condition     Condition
	Coverage      *float64
	Include       func() bool
	Namespace     *Namespace
	Force         *int
	HashAttribute string
	HashVersion   int
	Active        bool
	Status        ExperimentStatus
	URL           *regexp.Regexp
	Groups        []string
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

// WithWeights set the weights for an experiment.
func (exp *Experiment) WithWeights(weights ...float64) *Experiment {
	exp.Weights = weights
	return exp
}

// WithCondition sets the condition for an experiment.
func (exp *Experiment) WithCondition(condition Condition) *Experiment {
	exp.Condition = condition
	return exp
}

// WithCoverage sets the coverage for an experiment.
func (exp *Experiment) WithCoverage(coverage float64) *Experiment {
	exp.Coverage = &coverage
	return exp
}

// WithInclude sets the inclusion function for an experiment.
func (exp *Experiment) WithIncludeFunction(include func() bool) *Experiment {
	exp.Include = include
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

// WithHashVersion sets the hash version for an experiment.
func (exp *Experiment) WithHashVersion(hashVersion int) *Experiment {
	exp.HashVersion = hashVersion
	return exp
}

// WithActive sets the enabled flag for an experiment.
func (exp *Experiment) WithActive(active bool) *Experiment {
	exp.Active = active
	return exp
}

// WithStatus sets the status for an experiment.
func (exp *Experiment) WithStatus(status ExperimentStatus) *Experiment {
	exp.Status = status
	return exp
}

// WithGroups sets the groups for an experiment.
func (exp *Experiment) WithGroups(groups ...string) *Experiment {
	exp.Groups = groups
	return exp
}

// WithURL sets the URL for an experiment.
func (exp *Experiment) WithURL(url *regexp.Regexp) *Experiment {
	exp.URL = url
	return exp
}

// ParseExperiment creates an Experiment value from raw JSON input.
func ParseExperiment(data []byte) *Experiment {
	dict := make(map[string]interface{})
	err := json.Unmarshal(data, &dict)
	if err != nil {
		logError("Failed parsing JSON input", "Experiment")
		return nil
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
			key, ok := jsonString(v, "Experiment", "key")
			if !ok {
				return nil
			}
			exp.Key = key
			gotKey = true
		case "variations":
			exp = exp.WithVariations(BuildFeatureValues(v)...)
		case "ranges":
			ranges, ok := jsonRangeArray(v, "Experiment", "ranges")
			if !ok {
				return nil
			}
			exp = exp.WithRanges(ranges...)
		case "meta":
			meta, ok := jsonVariationMetaArray(v, "Experiment", "meta")
			if !ok {
				return nil
			}
			exp = exp.WithMeta(meta...)
		case "filters":
			filters, ok := jsonFilterArray(v, "Experiment", "filters")
			if !ok {
				return nil
			}
			exp = exp.WithFilters(filters...)
		case "seed":
			seed, ok := jsonString(v, "FeatureRule", "seed")
			if !ok {
				return nil
			}
			exp = exp.WithSeed(seed)
		case "name":
			name, ok := jsonString(v, "FeatureRule", "name")
			if !ok {
				return nil
			}
			exp = exp.WithName(name)
		case "phase":
			phase, ok := jsonString(v, "FeatureRule", "phase")
			if !ok {
				return nil
			}
			exp = exp.WithPhase(phase)
		case "weights":
			weights, ok := jsonFloatArray(v, "Experiment", "weights")
			if !ok {
				return nil
			}
			exp = exp.WithWeights(weights...)
		case "active":
			active, ok := jsonBool(v, "Experiment", "active")
			if !ok {
				return nil
			}
			exp = exp.WithActive(active)
		case "coverage":
			coverage, ok := jsonFloat(v, "Experiment", "coverage")
			if !ok {
				return nil
			}
			exp = exp.WithCoverage(coverage)
		case "condition":
			tmp, ok := v.(map[string]interface{})
			if !ok {
				logError("Invalid JSON data type", "Experiment", "condition")
				continue
			}
			cond := BuildCondition(tmp)
			if cond == nil {
				logError("Invalid condition in JSON experiment data")
			} else {
				exp = exp.WithCondition(cond)
			}
		case "namespace":
			namespace := BuildNamespace(v)
			if namespace == nil {
				return nil
			}
			exp = exp.WithNamespace(namespace)
		case "force":
			force, ok := jsonInt(v, "Experiment", "force")
			if !ok {
				return nil
			}
			exp = exp.WithForce(force)
		case "hashAttribute":
			hashAttribute, ok := jsonString(v, "Experiment", "hashAttribute")
			if !ok {
				return nil
			}
			exp = exp.WithHashAttribute(hashAttribute)
		case "hashVersion":
			hashVersion, ok := jsonInt(v, "Experiment", "hashVersion")
			if !ok {
				return nil
			}
			exp.HashVersion = hashVersion
		default:
			logWarn("Unknown key in JSON data", "Experiment", k)
		}
	}
	if !gotKey {
		logWarn("Key not set in JSON experiment data")
	}
	return exp
}

func (exp *Experiment) applyOverride(override *ExperimentOverride) *Experiment {
	newExp := *exp
	if override.Condition != nil {
		newExp.Condition = override.Condition
	}
	if override.Weights != nil {
		newExp.Weights = override.Weights
	}
	if override.Active != nil {
		newExp.Active = *override.Active
	}
	if override.Status != nil {
		newExp.Status = *override.Status
	}
	if override.Force != nil {
		newExp.Force = override.Force
	}
	if override.Coverage != nil {
		newExp.Coverage = override.Coverage
	}
	if override.Groups != nil {
		newExp.Groups = override.Groups
	}
	if override.Namespace != nil {
		newExp.Namespace = override.Namespace
	}
	if override.URL != nil {
		newExp.URL = override.URL
	}
	return &newExp
}

func experimentFromFeatureRule(id string, rule *FeatureRule) *Experiment {
	exp := NewExperiment(id).WithVariations(rule.Variations...)
	if rule.Key != "" {
		exp.Key = rule.Key
	}
	if rule.Coverage != nil {
		exp = exp.WithCoverage(*rule.Coverage)
	}
	if rule.Weights != nil {
		tmp := make([]float64, len(rule.Weights))
		copy(tmp, rule.Weights)
		exp = exp.WithWeights(tmp...)
	}
	if rule.HashAttribute != "" {
		exp = exp.WithHashAttribute(rule.HashAttribute)
	}
	if rule.Namespace != nil {
		val := Namespace{rule.Namespace.ID, rule.Namespace.Start, rule.Namespace.End}
		exp = exp.WithNamespace(&val)
	}
	if rule.Meta != nil {
		exp = exp.WithMeta(rule.Meta...)
	}
	if rule.Ranges != nil {
		exp = exp.WithRanges(rule.Ranges...)
	}
	if rule.Name != "" {
		exp = exp.WithName(rule.Name)
	}
	if rule.Phase != "" {
		exp = exp.WithPhase(rule.Phase)
	}
	if rule.Seed != "" {
		exp = exp.WithSeed(rule.Seed)
	}
	if rule.HashVersion != 0 {
		exp = exp.WithHashVersion(rule.HashVersion)
	}
	if rule.Filters != nil {
		exp = exp.WithFilters(rule.Filters...)
	}
	return exp
}
