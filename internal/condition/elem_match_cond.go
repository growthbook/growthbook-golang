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

func (c ElemMatchCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return c.eval3(actual, groups, visited) == pass
}

func (c ElemMatchCond) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	arr, ok := actual.(value.ArrValue)
	if !ok {
		return fail
	}
	res := fail
	for _, v := range arr {
		// Only null elements are skipped; 0, "" and false are still tested
		if v.Type() == value.NullType {
			continue
		}
		switch eval3(c.cond, v, groups, visited) {
		case pass:
			return pass
		case unknown:
			res = unknown
		}
	}
	return res
}
