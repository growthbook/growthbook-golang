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
	cond := NewInGroupCond(group)
	return NotCond{cond}
}

func (c InGroupCond) Eval(actual value.Value, groups SavedGroups) bool {
	if arr, ok := groups[c.group]; ok {
		return isIn(actual, arr)
	}
	return false
}
