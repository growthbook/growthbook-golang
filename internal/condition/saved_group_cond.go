package condition

import (
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// visitedGroups tracks saved-group IDs on the current evaluation branch to stop
// recursive reference cycles. It is never stored in the shared payload.
type visitedGroups []string

type savedGroupCond struct {
	id           string
	overridePath []string // nil uses the entry's attribute; [""] targets an empty key.
}

func (c savedGroupCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return c.eval3(actual, groups, visited) == pass
}

func (c savedGroupCond) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	group, ok := groups[c.id].(savedGroup)
	if !ok || group.cond == nil {
		return fail
	}
	for _, id := range visited {
		if id == c.id {
			return fail
		}
	}
	if group.newType {
		return unknown
	}
	if group.kind == "remote" {
		return resultOf(c.inRemoteGroup(actual, group))
	}
	// Each caller retains its slice length, so siblings do not inherit this ID.
	next := append(visited, c.id)
	// Overrides apply only to lists, without modifying the shared definition.
	if c.overridePath != nil && group.membership != nil {
		return resultOf((savedGroupListCond{path: c.overridePath, membership: *group.membership}).Eval(actual, groups, next))
	}
	// The group's list/condition evaluator was selected when the payload loaded.
	return eval3(group.cond, actual, groups, next)
}

// savedGroupListCond evaluates a v2 list group by looking up its attribute path
// and checking the value against the group's list using $in semantics.
type savedGroupListCond struct {
	path       []string
	membership InCond
}

func (c savedGroupListCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return c.membership.Eval(value.Path(actual, c.path...), groups, visited)
}

// inRemoteGroup checks the user's resolved remote group ids, which live outside
// the payload, in the __gb_remoteGroupIds attribute. A reference through
// another attribute needs a derived group, which isn't supported yet.
func (c savedGroupCond) inRemoteGroup(actual value.Value, group savedGroup) bool {
	if c.overridePath != nil && (group.attributeKey == nil || strings.Join(c.overridePath, ".") != *group.attributeKey) {
		return false
	}
	return hasRemoteGroupID(value.Path(actual, RemoteGroupIDsAttribute), c.id)
}

// hasRemoteGroupID reports whether ids is an array holding the string id
func hasRemoteGroupID(ids value.Value, id string) bool {
	arr, ok := ids.(value.ArrValue)
	if !ok {
		return false
	}
	for _, v := range arr {
		if s, ok := v.(value.StrValue); ok && string(s) == id {
			return true
		}
	}
	return false
}
