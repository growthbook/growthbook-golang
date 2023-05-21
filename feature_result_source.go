package growthbook

// FeatureResultSource is an enumerated type representing the source
// of a FeatureResult.
type FeatureResultSource uint

// FeatureResultSource values.
const (
	UnknownResultSource FeatureResultSource = iota + 1
	DefaultValueResultSource
	ForceResultSource
	ExperimentResultSource
	OverrideResultSource
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
	case "override":
		return OverrideResultSource
	default:
		return UnknownResultSource
	}
}
