package growthbook

import "encoding/json"

// FeatureRule overrides the default value of a Feature.
type FeatureRule struct {
	ID            string          `json:",omitempty"`
	Condition     *Condition      `json:"condition,omitempty"`
	Force         FeatureValue    `json:"force,omitempty"`
	Variations    []FeatureValue  `json:"variations,omitempty"`
	Weights       []float64       `json:"weights,omitempty"`
	Key           string          `json:"key,omitempty"`
	HashAttribute string          `json:"hashAttribute,omitempty"`
	HashVersion   int             `json:"hashVersion,omitempty"`
	Range         *Range          `json:"range,omitempty"`
	Coverage      *float64        `json:"coverage,omitempty"`
	Namespace     *Namespace      `json:"namespace,omitempty"`
	Ranges        []Range         `json:"ranges,omitempty"`
	Meta          []VariationMeta `json:"meta,omitempty"`
	Filters       []Filter        `json:"filters,omitempty"`
	Seed          string          `json:"seed,omitempty"`
	Name          string          `json:"name,omitempty"`
	Phase         string          `json:"phase,omitempty"`
}

// Clone via JSON for simplicity.
func (r *FeatureRule) clone() *FeatureRule {
	data, err := json.Marshal(r)
	if err != nil {
		return nil
	}
	retval := FeatureRule{}
	err = json.Unmarshal(data, &retval)
	if err != nil {
		return nil
	}
	return &retval
}
