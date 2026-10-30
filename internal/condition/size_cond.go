package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// SizeCond checks length of field array
type SizeCond struct {
	cond Condition
}

func NewSizeCond(cond Condition) SizeCond {
	return SizeCond{cond}
}

func (c SizeCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return c.eval3(actual, groups, visited) == pass
}

func (c SizeCond) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	if arr, ok := actual.(value.ArrValue); ok {
		return eval3(c.cond, value.New(len(arr)), groups, visited)
	}
	return fail
}
