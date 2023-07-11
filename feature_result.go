package growthbook

// FeatureResult is the result of evaluating a feature.
type FeatureResult struct {
	Value            FeatureValue        `json:"value,omitempty"`
	Source           FeatureResultSource `json:"source,omitempty"`
	On               bool                `json:"on,omitempty"`
	Off              bool                `json:"off,omitempty"`
	RuleID           string
	Experiment       *Experiment `json:"experiment,omitempty"`
	ExperimentResult *Result     `json:"experimentResult,omitempty"`
}
