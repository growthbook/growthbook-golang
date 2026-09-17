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
	if arr, ok := groups[c.group].(value.ArrValue); ok {
		for _, v := range arr {
			if value.Equal(actual, v) {
				return true
			}
		}
	}
	return false
}

type notInGroupCond struct {
	group string
}

func (c notInGroupCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	if entry, present := groups[c.group]; present {
		if _, legacy := entry.(value.ArrValue); !legacy {
			return false
		}
	}
	return !NewInGroupCond(c.group).Eval(actual, groups, visited)
}
