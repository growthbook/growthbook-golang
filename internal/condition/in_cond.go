package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// InOp checks if value is in array
type InCond struct {
	expected value.ArrValue
}

func NewInCond(arg value.ArrValue) InCond {
	return InCond{arg}
}

func NewNotInCond(arg value.ArrValue) Condition {
	cond := NewInCond(arg)
	return NotCond{cond}
}

func (c InCond) Eval(actual value.Value, _ SavedGroups) bool {
	return isIn(actual, c.expected)
}
