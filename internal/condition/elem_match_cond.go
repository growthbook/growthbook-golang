package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// ElemMatchCond checks at least one element of field value array
// matches the expected condition
type ElemMatchCond struct {
	cond Condition
}

func NewElemMatchCond(cond Condition) ElemMatchCond {
	return ElemMatchCond{cond}
}

func (c ElemMatchCond) Eval(actual value.Value, groups SavedGroups) bool {
	arr, ok := actual.(value.ArrValue)
	if !ok {
		return false
	}
	for _, v := range arr {
		if c.cond.Eval(v, groups) {
			return true
		}
	}
	return false
}
