package growthbook

import (
	"encoding/json"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

type ParentCondition struct {
	Id        string         `json:"id"`
	Condition condition.Base `json:"condition"`
	Gate      bool           `json:"gate"`
}

// UnmarshalJSON decodes a prerequisite. A malformed condition (for example
// `{"$not": null}`) never matches instead of failing the whole payload,
// matching the JS SDK.
func (p *ParentCondition) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID        string          `json:"id"`
		Condition json.RawMessage `json:"condition"`
		Gate      bool            `json:"gate"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = ParentCondition{Id: raw.ID, Gate: raw.Gate}
	if len(raw.Condition) == 0 || string(raw.Condition) == "null" {
		return nil
	}
	// A malformed condition parses to one that never matches
	return json.Unmarshal(raw.Condition, &p.Condition)
}
