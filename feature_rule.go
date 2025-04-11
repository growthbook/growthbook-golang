package growthbook

import "github.com/growthbook/growthbook-golang/internal/condition"

type FeatureRule struct {
	// Optional rule id, reserved for future use
	Id string `json:"id"`
	// Optional targeting condition
	Condition condition.Base `json:"condition"`
	// Each item defines a prerequisite where a condition must evaluate against a parent feature's value (identified by id).
	// If gate is true, then this is a blocking feature-level prerequisite; otherwise it applies to the current rule only.
	ParentConditions []ParentCondition `json:"parentConditions"`
	// What percent of users should be included in the experiment (between 0 and 1, inclusive)
	Coverage *float64 `json:"coverage"`
	// Immediately force a specific value (ignore every other option besides condition and coverage)
	Force FeatureValue `json:"force"`
	// Run an experiment (A/B test) and randomly choose between these variations
	Variations []FeatureValue `json:"variations"`
	// The globally unique tracking key for the experiment (default to the feature key)
	Key string `json:"key"`
	// How to weight traffic between variations. Must add to 1.
	Weights []float64 `json:"weights"`
	// Adds the experiment to a namespace
	Namespace *Namespace `json:"namespace"`
	// What user attribute should be used to assign variations (defaults to id)
	HashAttribute string `json:"hashAttribute"`
	// When using sticky bucketing, can be used as a fallback to assign variations
	FallbackAttribute string `json:"fallbackAttribute"`
	// The hash version to use (default to 1)
	HashVersion int `json:"hashVersion"`
	// A more precise version of coverage
	Range *BucketRange `json:"range"`
	// Ranges for experiment variations
	Ranges []BucketRange `json:"ranges"`
	// Meta info about the experiment variations
	Meta []VariationMeta `json:"meta"`
	// Slice of filters to apply to the rule
	Filters []Filter `json:"filters"`
	// Seed to use for hashing
	Seed string `json:"seed"`
	//Human-readable name for the experiment
	Name string `json:"name"`
	// The phase id of the experiment
	Phase string `json:"phase"`
}
