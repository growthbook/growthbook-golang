package growthbook

// Result records the result of running an Experiment given a specific
// Context.
type Result struct {
	Value         FeatureValue
	VariationID   int
	Key           string
	Name          string
	Bucket        *float64
	Passthrough   bool
	InExperiment  bool
	HashUsed      bool
	HashAttribute string
	HashValue     string
	FeatureID     string
}
