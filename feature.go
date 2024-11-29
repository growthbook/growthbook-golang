package growthbook

// Feature has a default value plus rules than can override the
// default.
type Feature struct {
	// DefaultValue is optional default value
	DefaultValue FeatureValue `json:"defaultValue"`
	//Rules determine when and how the [DefaultValue] gets overridden
	Rules []FeatureRule `json:"rules"`
}

// Map of [Feature]. Keys are string ids for the features.
// Values are pointers to [Feature] structs.
type FeatureMap map[string]*Feature
