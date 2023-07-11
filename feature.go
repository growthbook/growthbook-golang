package growthbook

// FeatureValue is a wrapper around an arbitrary type representing the
// value of a feature. Features can return any kinds of values, so
// this is an alias for interface{}.
type FeatureValue interface{}

// Feature has a default value plus rules than can override the
// default.
type Feature struct {
	DefaultValue FeatureValue   `json:"defaultValue"`
	Rules        []*FeatureRule `json:"rules"`
}
