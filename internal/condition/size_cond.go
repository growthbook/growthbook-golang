package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// SizeCond checks length of field array
type SizeCond struct {
	cond Condition
}

func NewSizeCond(cond Condition) SizeCond {
	return SizeCond{cond}
}

func (c SizeCond) Eval(actual value.Value, groups SavedGroups) bool {
	if arr, ok := actual.(value.ArrValue); ok {
		return c.cond.Eval(value.New(len(arr)), groups)
	}
	return false
}
