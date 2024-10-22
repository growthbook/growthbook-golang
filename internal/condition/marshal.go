package condition

import (
	"encoding/json"
	"fmt"
)

func (base *Base) UnmarshalJSON(data []byte) error {
	m := map[string]json.RawMessage{}
	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	err = buildBase(m, base)
	if err != nil {
		return err
	}
	return nil
}

func buildBase(m map[string]json.RawMessage, base *Base) error {
	for k, raw := range m {
		cond, err := buildCond(k, raw)
		if err != nil {
			return err
		}
		*base = append(*base, cond)
	}
	return nil
}

func buildCond(k string, raw json.RawMessage) (Condition, error) {
	switch k {
	case "$and":
		return buildAnd(raw)
	case "$or":
		return buildOr(raw)
	case "$not":
		return buildNot(raw)
	case "$nor":
		return buildNor(raw)
	default:
		return nil, fmt.Errorf("not implemented")
	}
}

func buildAnd(raw json.RawMessage) (And, error) {
	cond := And{}
	err := json.Unmarshal(raw, &cond)
	if err != nil {
		return nil, fmt.Errorf("Error parsing `and` condition: %w", err)
	}
	return cond, nil
}

func buildOr(raw json.RawMessage) (Or, error) {
	cond := Or{}
	err := json.Unmarshal(raw, &cond)
	if err != nil {
		return nil, fmt.Errorf("Error parsing `or` condition. %w", err)
	}
	return cond, nil
}

func buildNor(raw json.RawMessage) (Nor, error) {
	cond := Nor{}
	err := json.Unmarshal(raw, &cond)
	if err != nil {
		return nil, fmt.Errorf("Error parsing `nor` condition. %w", err)
	}
	return cond, nil
}

func buildNot(raw json.RawMessage) (Not, error) {
	cond := Base{}
	err := json.Unmarshal(raw, &cond)
	if err != nil {
		return nil, fmt.Errorf("Error parsing `not` condition. %w", err)
	}
	return Not(cond), nil
}
