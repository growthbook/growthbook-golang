package growthbook

// FeatureValue is a wrapper around an arbitrary type representing the
// value of a feature. Features can return any kinds of values, so
// this is an alias for any.
type FeatureValue any

// Feature has a default value plus rules than can override the
// default.
type Feature struct {
	// DefaultValue is optional default value
	DefaultValue FeatureValue `json:"defaultValue"`
	//Rules determine when and how the [DefaultValue] gets overridden
	Rules []*FeatureRule `json:"rules"`
}

// Map of [Feature]. Keys are string ids for the features.
// Values are pointers to [Feature] structs.
type FeatureMap map[string]*Feature

func (features FeatureMap) Eval(key string) *FeatureResult {
	feature := features[key]
	if feature == nil {
		return getFeatureResult(nil, UnknownFeatureResultSource, nil, nil)
	}

	return getFeatureResult(feature.DefaultValue, DefaultValueResultSource, nil, nil)
}
