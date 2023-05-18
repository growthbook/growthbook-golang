package growthbook

// FeatureResultSource is an enumerated type representing the source
// of a FeatureResult.
type FeatureResultSource uint

// FeatureResultSource values.
const (
	UnknownFeatureResultSource FeatureResultSource = iota + 1
	DefaultValueResultSource
	ForceResultSource
	ExperimentResultSource
)

// ParseFeatureResultSource creates a FeatureResultSource value from
// its string representation.
func ParseFeatureResultSource(source string) FeatureResultSource {
	switch source {
	case "", "defaultValue":
		return DefaultValueResultSource
	case "force":
		return ForceResultSource
	case "experiment":
		return ExperimentResultSource
	default:
		return UnknownFeatureResultSource
	}
}
