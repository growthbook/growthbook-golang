package condition

import (
	"strconv"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// visitedGroups tracks saved-group IDs on the current evaluation branch to stop
// recursive reference cycles. It is never stored in the shared payload.
type visitedGroups map[string]struct{}

type savedGroupCond struct {
	id string
}

func (c savedGroupCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	group, ok := groups[c.id].(savedGroup)
	if !ok || group.cond == nil {
		return false
	}
	if _, cycle := visited[c.id]; cycle {
		return false
	}
	// Copy before descending so siblings can independently resolve the same ID.
	next := make(visitedGroups, len(visited)+1)
	for id := range visited {
		next[id] = struct{}{}
	}
	next[c.id] = struct{}{}
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
	for _, part := range c.path {
		switch current := actual.(type) {
		case value.ObjValue:
			actual = current[part]
		case value.ArrValue:
			if part == "length" {
				actual = value.Num(len(current))
				continue
			}
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != part {
				actual = nil
			} else {
				actual = current[index]
			}
		default:
			actual = nil
		}
		if actual == nil {
			actual = value.Null()
			break
		}
	}
	return c.membership.Eval(actual, groups, visited)
}
