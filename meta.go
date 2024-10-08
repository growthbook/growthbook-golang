package growthbook

// VariationMeta info about an experiment variation.
type VariationMeta struct {
	// Key is a unique key for this variation.
	Key string `json:"key"`
	// Name is a human-readable name for this variation.
	Name string `json:"name"`
	// Passthrough used to implement holdout groups
	Passthrough bool `json:"passthrough"`
}
