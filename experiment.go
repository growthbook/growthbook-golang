package growthbook

import "github.com/growthbook/growthbook-golang/internal/condition"

type ExperimentStatus string

const (
	DraftStatus   ExperimentStatus = "draft"
	RunningStatus ExperimentStatus = "running"
	StoppedStatus ExperimentStatus = "stopped"
)

// Experiment defines a single experiment.
type Experiment struct {
	// The globally unique identifier for the experiment
	Key string `json:"key"`
	// The different variations to choose between
	Variations []FeatureValue `json:"variations"`
	// How to weight traffic between variations. Must add to 1.
	Weights []float64 `json:"weights"`
	// If set to false, always return the control (first variation)
	Active bool `json:"active"`
	// What percent of users should be included in the experiment (between 0 and 1, inclusive)
	Coverage *float64 `json:"coverage"`
	// Array of ranges, one per variation
	Ranges []BucketRange `json:"ranges"`
	// Optional targeting condition
	Condition *condition.Base `json:"condition"`
	// Adds the experiment to a namespace
	Namespace Namespace `json:"namespace"`
	// All users included in the experiment will be forced into the specific variation index
	Force int `json:"force"`
	// What user attribute should be used to assign variations (defaults to id)
	HashAttribute string `json:"hashAttribute"`
	// When using sticky bucketing, can be used as a fallback to assign variations
	FallbackAttribute string `json:"fallbackAttribute"`
	// The hash version to use (default to 1)
	HashVersion int `json:"hashVersion"`
	// Meta info about the variations
	Meta []VariationMeta `json:"meta"`
	// Array of filters to apply
	Filters []Filter `json:"filters"`
	// The hash seed to use
	Seed string `json:"seed"`
	// Human-readable name for the experiment
	Name string `json:"name"`
	// Id of the current experiment phase
	Phase string `json:"phase"`
	// If true, sticky bucketing will be disabled for this experiment.
	// (Note: sticky bucketing is only available if a StickyBucketingService is provided in the Context)
	DisableStickyBucketing bool `json:"disableStickyBucketing"`
	// An sticky bucket version number that can be used to force a re-bucketing of users (default to 0)
	BucketVersion int `json:"bucketVersion"`
	// Any users with a sticky bucket version less than this will be excluded from the experiment
	MinBucketVersion int `json:"minBucketVersion"`
}

// NewExperiment creates an experiment with default settings: active,
// but all other fields empty.
func NewExperiment(key string) *Experiment {
	return &Experiment{
		Key:    key,
		Active: true,
	}
}

func experimentFromFeatureRule(id string, rule *FeatureRule) *Experiment {
	exp := &Experiment{
		Key:           id,
		Variations:    rule.Variations,
		Coverage:      rule.Coverage,
		Weights:       rule.Weights,
		HashAttribute: rule.HashAttribute,
		Namespace:     rule.Namespace,
		Meta:          rule.Meta,
		Ranges:        rule.Ranges,
		Name:          rule.Name,
		Phase:         rule.Phase,
		Seed:          rule.Seed,
		HashVersion:   rule.HashVersion,
		Filters:       rule.Filters,
	}
	return exp
}
