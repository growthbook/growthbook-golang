package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// InGroupCond checks if value is in saved group
type InGroupCond struct {
	group string
}

func NewInGroupCond(group string) InGroupCond {
	return InGroupCond{group}
}

func NewNotInGroupCond(group string) Condition {
	return notInGroupCond{group: group}
}

func (c InGroupCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	membership, ok := savedGroupMembership(groups, c.group)
	return ok && membership.Eval(actual, groups, visited)
}

type notInGroupCond struct {
	group string
}

func (c notInGroupCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	membership, ok := savedGroupMembership(groups, c.group)
	return ok && !membership.Eval(actual, groups, visited)
}

func savedGroupMembership(groups SavedGroups, id string) (InCond, bool) {
	entry, present := groups[id]
	if !present {
		// Only missing IDs behave as empty lists; malformed entries fail closed.
		return InCond{}, true
	}
	switch group := entry.(type) {
	case value.ArrValue:
		return InCond{expected: group}, true
	case savedGroup:
		if group.membership != nil {
			return *group.membership, true
		}
	}
	return InCond{}, false
}
