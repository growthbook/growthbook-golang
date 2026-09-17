package condition

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// SavedGroups holds legacy value arrays and parsed v2 definitions. Malformed
// entries are retained so legacy exclusion operators can distinguish them from
// an absent group. Definitions are compiled once when loading the payload.
type SavedGroups map[string]any

type savedGroup struct {
	raw  json.RawMessage
	cond Condition
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
		if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
			var values []any
			if err := json.Unmarshal(raw, &values); err != nil {
				return err
			}
			parsed[id] = value.New(values)
		} else {
			parsed[id] = savedGroup{raw: raw, cond: parseSavedGroup(raw)}
		}
	}
	*sg = parsed
	return nil
}

func parseSavedGroup(raw json.RawMessage) Condition {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return False{}
	}
	var kind string
	if err := json.Unmarshal(entry["type"], &kind); err != nil {
		return False{}
	}
	// Select the evaluator at load time: list membership or a parsed condition.
	switch kind {
	case "list":
		var attributeKey *string
		if err := json.Unmarshal(entry["attributeKey"], &attributeKey); err != nil || attributeKey == nil {
			return False{}
		}
		if !bytes.HasPrefix(bytes.TrimSpace(entry["values"]), []byte("[")) {
			return False{}
		}
		var values []any
		if err := json.Unmarshal(entry["values"], &values); err != nil {
			return False{}
		}
		return savedGroupListCond{
			path:       strings.Split(*attributeKey, "."),
			membership: NewInCond(value.New(values).(value.ArrValue)),
		}
	case "condition":
		if !bytes.HasPrefix(bytes.TrimSpace(entry["condition"]), []byte("{")) {
			return False{}
		}
		var cond Base
		if err := json.Unmarshal(entry["condition"], &cond); err != nil {
			return False{}
		}
		// Reuse the compiled condition, not Base.Eval, which starts a new
		// evaluation and would discard the caller's cycle guard.
		return cond.cond
	default:
		return False{}
	}
}
