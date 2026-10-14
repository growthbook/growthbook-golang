package growthbook

type ExperimentResult struct {
	// Whether or not the user is part of the experiment
	InExperiment bool `json:"inExperiment"`
	// The array index of the assigned variation
	VariationId int `json:"variationId"`
	// The array value of the assigned variation
	Value FeatureValue `json:"value"`
	// If a hash was used to assign a variation
	HashUsed bool `json:"hashUsed"`
	// The user attribute used to assign a variation
	HashAttribute string `json:"hashAttribute"`
	// The value of hash attribute
	HashValue string `json:"hashValue"`
	// The id of the feature (if any) that the experiment came from
	FeatureId string `json:"featureId"`
	// The unique key for the assigned variation
	Key string `json:"key"`
	// The hash value used to assign a variation (float from 0 to 1)
	Bucket *float64 `json:"bucket"`
	// The human-readable name of the assigned variation
	Name string `json:"name"`
	// Used for holdout groups
	Passthrough bool `json:"passthrough"`
	// If sticky bucketing was used to assign a variation
	StickyBucketUsed bool `json:"stickyBucketUsed"`
	// The contextual bandit leaf used for this assignment
	LeafId *int `json:"leafId,omitempty"`
	// The variation weights the contextual bandit assigned with
	VariationWeights []float64 `json:"variationWeights,omitempty"`
	// The version of the contextual bandit definition used
	BanditVersion *int `json:"banditVersion,omitempty"`
}
