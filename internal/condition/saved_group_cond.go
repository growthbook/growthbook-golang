package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// visitedGroups tracks saved-group IDs on the current evaluation branch to stop
// recursive reference cycles. It is never stored in the shared payload.
type visitedGroups []string

type savedGroupCond struct {
	id           string
	overridePath []string // nil uses the entry's attribute; [""] targets an empty key.
}

func (c savedGroupCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	group, ok := groups[c.id].(savedGroup)
	if !ok || group.cond == nil {
		return false
	}
	for _, id := range visited {
		if id == c.id {
			return false
		}
	}
	// Each caller retains its slice length, so siblings do not inherit this ID.
	next := append(visited, c.id)
	// Overrides apply only to lists, without modifying the shared definition.
	if c.overridePath != nil && group.membership != nil {
		return (savedGroupListCond{path: c.overridePath, membership: *group.membership}).Eval(actual, groups, next)
	}
	// The group's list/condition evaluator was selected when the payload loaded.
	return group.cond.Eval(actual, groups, next)
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
