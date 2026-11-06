package condition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// RemoteGroupIDsAttribute is the attribute conditions read a user's remote
// saved groups from. Internal attributes start with `__gb_`, matching
// GrowthBook's back end.
const RemoteGroupIDsAttribute = "__gb_remoteGroupIds"

// RemoteGroup is a remote saved group from the payload: its members live
// outside the payload, and the app resolves which of them a user is in.
type RemoteGroup struct {
	ID           string
	AttributeKey string
	// ValuesFrom is the remote group whose ids a derived group checks
	ValuesFrom string
}

// RemoteGroups returns the payload's remote saved groups, sorted by id. Only
// v2 payloads carry this metadata. An entry without an attribute gives a
// resolver nothing to look up, so it's left out and its rules fail closed.
func (sg SavedGroups) RemoteGroups() []RemoteGroup {
	var groups []RemoteGroup
	for id, entry := range sg {
		group, ok := entry.(savedGroup)
		if !ok || group.kind != "remote" || group.attributeKey == nil {
			continue
		}
		groups = append(groups, RemoteGroup{ID: id, AttributeKey: *group.attributeKey, ValuesFrom: group.valuesFrom})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups
}

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
	// newType marks an object entry whose type is not "list", "condition" or
	// "remote": a group type added after this SDK was built. Conditions that
	// refer to it are unknown rather than failing, since the client evaluating
	// the payload may support it.
	newType bool
	// kind, attributeKey and conditionJSON are what pruning reads, decoded
	// once here rather than from raw on every reference
	kind          string
	attributeKey  *string
	conditionJSON map[string]any
	// valuesFrom is the source group of a derived remote group
	valuesFrom string
}

func (g savedGroup) MarshalJSON() ([]byte, error) { return g.raw, nil }

// JSONValue returns the groups as decoded JSON in the exact shape they were
// loaded from: a v2 entry as its raw JSON, and a legacy list as its values,
// so an empty list stays [] and a null member stays null.
func (sg SavedGroups) JSONValue() (map[string]any, error) {
	out := make(map[string]any, len(sg))
	for id, group := range sg {
		switch g := group.(type) {
		case savedGroup:
			var v any
			if err := json.Unmarshal(g.raw, &v); err != nil {
				return nil, fmt.Errorf("saved group %q: %w", id, err)
			}
			out[id] = v
		case value.Value:
			out[id] = value.ToAny(g)
		default:
			out[id] = group
		}
	}
	return out, nil
}

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
	if entry == nil {
		return group
	}
	var kind string
	if err := json.Unmarshal(entry["type"], &kind); err != nil || (kind != "list" && kind != "condition" && kind != "remote") {
		group.newType = true
		return group
	}
	group.kind = kind
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
		group.attributeKey = attributeKey
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
		if err := json.Unmarshal(entry["condition"], &group.conditionJSON); err != nil {
			return group
		}
		// Reuse the compiled condition, not Base.Eval, which starts a new
		// evaluation and would discard the caller's cycle guard.
		group.cond = cond.cond
	case "remote":
		// The members live outside the payload; a $savedGroup reference checks
		// the user's resolved ids (see savedGroupCond)
		var attributeKey *string
		if err := json.Unmarshal(entry["attributeKey"], &attributeKey); err == nil {
			group.attributeKey = attributeKey
		}
		var valuesFrom string
		if err := json.Unmarshal(entry["valuesFrom"], &valuesFrom); err == nil {
			group.valuesFrom = valuesFrom
		}
	}
	return group
}
