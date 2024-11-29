package growthbook

// FeatureValue is a wrapper around an arbitrary type representing the
// value of a feature.
type FeatureValue any

// This function imitates Javascript's "truthiness" evaluation for Go
// values of unknown type.
func truthy(v FeatureValue) bool {
	if v == nil {
		return false
	}
	switch r := v.(type) {
	case string:
		return r != ""
	case bool:
		return r
	case int:
		return r != 0
	case uint:
		return r != 0
	case float32:
		return r != 0
	case float64:
		return r != 0
	}
	return true
}
