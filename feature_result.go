package growthbook

import "encoding/json"

type ExperimentWithResult struct {
	Experiment       *Experiment `json:"experiment,omitempty"`
	ExperimentResult *Result     `json:"experimentResult,omitempty"`
}

// FeatureResult is the result of evaluating a feature.
type FeatureResult struct {
	Value            FeatureValue        `json:"value,omitempty"`
	Source           FeatureResultSource `json:"source,omitempty"`
	On               bool                `json:"on,omitempty"`
	Off              bool                `json:"off,omitempty"`
	RuleID           string
	ExperimentResult *ExperimentWithResult
}

// UnmarshalJSON deserializes feature result data from JSON, with
// custom handling for the experiment and experiment result fields,
// which we bundle up into a single value.

func (r *FeatureResult) UnmarshalJSON(data []byte) error {
	type Alias FeatureResult
	tmp := &struct {
		*Alias
		ExperimentResult any
	}{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	var ewr ExperimentWithResult
	err = json.Unmarshal(data, &ewr)
	if err != nil {
		return err
	}
	r.Value = tmp.Value
	r.Source = tmp.Source
	r.On = tmp.On
	r.Off = tmp.Off
	r.RuleID = tmp.RuleID
	if ewr.Experiment == nil && ewr.ExperimentResult == nil {
		r.ExperimentResult = nil
	} else {
		r.ExperimentResult = &ewr
	}
	return nil
}
