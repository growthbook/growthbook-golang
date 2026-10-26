package condition

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// errMalformed stops pruning a condition that can't be evaluated, which then
// stays as written: evaluating it fails as a whole, so no part of it can be
// decided.
var errMalformed = errors.New("malformed condition")

// Prune partially evaluates cond, decoded JSON, when only some attributes are
// known, e.g. on a server. Attributes missing from attrs are unknown. It
// returns a bool once the result can't depend on them, otherwise the condition
// minus every branch that is already decided.
func Prune(attrs value.ObjValue, cond any, groups SavedGroups) any {
	obj, ok := cond.(map[string]any)
	if !ok {
		return cond
	}
	res, err := pruneEntries(attrs, obj, groups, nil)
	if err != nil {
		return cond
	}
	return res
}

// PruneSavedGroup prunes the saved group with the given id. It returns true or
// false once decided, nil when an unknown attribute or an unsupported group
// type decides it, or, for a condition group, its condition minus every branch
// that is already decided.
func PruneSavedGroup(attrs value.ObjValue, id string, groups SavedGroups) any {
	res, err := pruneSavedGroup(attrs, map[string]any{"id": id}, groups, nil)
	if err != nil {
		return nil
	}
	return res
}

// pruneEntries returns true, false, or the entries still undecided. A nil
// entry result is an undecided leaf, which stays as written.
func pruneEntries(attrs value.ObjValue, cond map[string]any, groups SavedGroups, visited visitedGroups) (any, error) {
	out := map[string]any{}
	decided := false
	var firstErr error
	for k, v := range cond {
		var r any
		var err error
		switch k {
		case string(orOp), string(norOp), string(andOp):
			r, err = pruneList(attrs, v, groups, visited, k != string(andOp))
		case string(notOp):
			obj, ok := v.(map[string]any)
			if !ok {
				err = errMalformed
				break
			}
			r, err = pruneEntries(attrs, obj, groups, visited)
		case string(savedGroupOp):
			r, err = pruneSavedGroup(attrs, v, groups, visited)
			// Keep an undecided reference as is rather than inlining the group
			if _, ok := r.(bool); !ok {
				r = nil
			}
		default:
			r, err = pruneValue(attrs, k, v, groups, visited)
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// $not and $nor pass when what they hold fails
		if b, ok := r.(bool); ok && (k == string(notOp) || k == string(norOp)) {
			r = !b
		}
		switch r {
		case false:
			// One failing entry fails the whole condition
			decided = true
		case nil:
			out[k] = v
		case true:
		default:
			out[k] = r
		}
	}
	if decided {
		return false, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if len(out) > 0 {
		return out, nil
	}
	return true, nil
}

// pruneValue evaluates an attribute's condition when the attribute is known.
// It returns nil when the attribute is unknown, or when the condition uses an
// operator this SDK doesn't support, which the client's SDK may.
func pruneValue(attrs value.ObjValue, key string, v any, groups SavedGroups, visited visitedGroups) (any, error) {
	if !isKnown(attrs, key) {
		return nil, nil
	}
	cond, err := buildValueCond(value.New(v))
	if err != nil {
		return nil, errMalformed
	}
	switch eval3(cond, attrs[key], groups, visited) {
	case pass:
		return true, nil
	case fail:
		return false, nil
	}
	return nil, nil
}

// pruneList prunes an $or (or an $and) list. It returns the conditions still
// undecided, or the list's result once there are none.
func pruneList(attrs value.ObjValue, v any, groups SavedGroups, visited visitedGroups, or bool) (any, error) {
	conds, ok := v.([]any)
	if !ok {
		return nil, errMalformed
	}
	out := []any{}
	for _, c := range conds {
		obj, ok := c.(map[string]any)
		if !ok {
			return nil, errMalformed
		}
		r, err := pruneEntries(attrs, obj, groups, visited)
		if err != nil {
			return nil, err
		}
		// One passing condition decides an $or, and one failing condition an $and
		if or && r == true {
			return true, nil
		}
		if !or && r == false {
			return false, nil
		}
		if _, ok := r.(bool); !ok {
			out = append(out, r)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	// Every condition failed an $or, or passed an $and. As in evaluation, an
	// empty $or passes.
	if or {
		return len(conds) == 0, nil
	}
	return true, nil
}

// pruneSavedGroup resolves a $savedGroup reference like savedGroupCond, but
// returns nil for a list group whose attribute is unknown, and the pruned
// condition of a condition group. Anything unrecognized matches nobody.
func pruneSavedGroup(attrs value.ObjValue, ref any, groups SavedGroups, visited visitedGroups) (any, error) {
	reference, ok := ref.(map[string]any)
	if !ok {
		return false, nil
	}
	id, ok := reference["id"].(string)
	if !ok {
		return false, nil
	}
	for _, seen := range visited {
		if seen == id {
			return false, nil
		}
	}
	attributeKey, overridden := reference["attributeKey"]
	if _, isStr := attributeKey.(string); overridden && !isStr {
		// Falling back to the entry's attribute would check a different population
		return false, nil
	}
	group, ok := groups[id].(savedGroup)
	if !ok {
		// Absent, or a v1 bare array
		return false, nil
	}
	var entry map[string]any
	if err := json.Unmarshal(group.raw, &entry); err != nil || entry == nil {
		return false, nil
	}
	if group.newType {
		// A group type added after this SDK was built. The client SDK may support it.
		return nil, nil
	}
	switch entry["type"] {
	case "list":
		key, ok := attributeKey.(string)
		if !overridden {
			key, ok = entry["attributeKey"].(string)
		}
		values, isArr := entry["values"].([]any)
		if !ok || !isArr {
			return false, nil
		}
		if !isKnown(attrs, key) {
			return nil, nil
		}
		arr, _ := value.New(values).(value.ArrValue)
		return NewInCond(arr).Eval(attrs[key], groups, visited), nil
	case "condition":
		// A condition group has no single attribute, so any override is ignored
		cond, ok := entry["condition"].(map[string]any)
		if !ok {
			return false, nil
		}
		// Each caller retains its slice length, so siblings do not inherit this ID.
		return pruneEntries(attrs, cond, groups, append(visited, id))
	}
	return false, nil
}

// isKnown reports whether a condition's attribute is known. A nested path is
// unknown, since the server may only have part of the object.
func isKnown(attrs value.ObjValue, key string) bool {
	if strings.Contains(key, ".") {
		return false
	}
	_, ok := attrs[key]
	return ok
}
