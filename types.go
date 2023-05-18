package growthbook

// Attributes is an arbitrary JSON object containing user and request
// attributes.
type Attributes map[string]interface{}

// FeatureValue is a wrapper around an arbitrary type representing the
// value of a feature. Features can return any kinds of values, so
// this is an alias for interface{}.
type FeatureValue interface{}

// FeatureMap is a map of feature objects, keyed by string feature
// IDs.
type FeatureMap map[string]*Feature

// ForcedVariationsMap is a map that forces an Experiment to always
// assign a specific variation. Useful for QA.
//
// Keys are the experiment key, values are the array index of the
// variation.
type ForcedVariationsMap map[string]int
