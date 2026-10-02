package condition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// SavedGroups holds legacy value arrays and parsed v2 definitions. Malformed
// entries are retained so legacy exclusion operators can distinguish them from
// an absent group. Definitions are compiled once when loading the payload.
type SavedGroups map[string]any

// Normalize compiles native Go definitions using the JSON loading rules without
// changing the caller's map. Already-parsed entries are reused, not recompiled.
func (sg SavedGroups) Normalize() (SavedGroups, error) {
	if sg == nil {
		return nil, nil
	}
	parsed := make(SavedGroups, len(sg))
	for id, group := range sg {
		switch group.(type) {
		case savedGroup, value.ArrValue:
			parsed[id] = group
		default:
			raw, err := json.Marshal(group)
			if err != nil {
				return nil, fmt.Errorf("saved group %q: %w", id, err)
			}
			parsed[id], err = parseSavedGroupEntry(raw)
			if err != nil {
				return nil, fmt.Errorf("saved group %q: %w", id, err)
			}
		}
	}
	return parsed, nil
}

type savedGroup struct {
	raw        json.RawMessage
	cond       Condition
	membership *InCond
}

func (g savedGroup) MarshalJSON() ([]byte, error) { return g.raw, nil }

func (sg *SavedGroups) UnmarshalJSON(data []byte) error {
	var groups map[string]json.RawMessage
	if err := json.Unmarshal(data, &groups); err != nil {
		return err
	}
	if groups == nil {
		*sg = nil
		return nil
	}
	parsed := make(SavedGroups, len(groups))
	for id, raw := range groups {
		group, err := parseSavedGroupEntry(raw)
		if err != nil {
			return err
		}
		parsed[id] = group
	}
	*sg = parsed
	return nil
}

func parseSavedGroupEntry(raw json.RawMessage) (any, error) {
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		var values []any
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		return value.New(values), nil
	}
	return parseSavedGroup(raw), nil
}

func parseSavedGroup(raw json.RawMessage) savedGroup {
	group := savedGroup{raw: raw, cond: False{}}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return group
	}
	var kind string
	if err := json.Unmarshal(entry["type"], &kind); err != nil {
		return group
	}
	// Select the evaluator at load time: list membership or a parsed condition.
	switch kind {
	case "list":
		if !bytes.HasPrefix(bytes.TrimSpace(entry["values"]), []byte("[")) {
			return group
		}
		var values []any
		if err := json.Unmarshal(entry["values"], &values); err != nil {
			return group
		}
		// Legacy operators only need values; $savedGroup also needs an attribute.
		membership := NewInCond(value.New(values).(value.ArrValue))
		group.membership = &membership
		var attributeKey *string
		if err := json.Unmarshal(entry["attributeKey"], &attributeKey); err != nil || attributeKey == nil {
			return group
		}
		group.cond = savedGroupListCond{
			path:       strings.Split(*attributeKey, "."),
			membership: membership,
		}
	case "condition":
		if !bytes.HasPrefix(bytes.TrimSpace(entry["condition"]), []byte("{")) {
			return group
		}
		var cond Base
		if err := json.Unmarshal(entry["condition"], &cond); err != nil {
			return group
		}
		// Reuse the compiled condition, not Base.Eval, which starts a new
		// evaluation and would discard the caller's cycle guard.
		group.cond = cond.cond
	}
	return group
}
