package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// AllConds checks each condition is true for at least
// one array element.
type AllConds []Condition

func (cs AllConds) Eval(actual value.Value, groups SavedGroups) bool {
	arr, ok := actual.(value.ArrValue)
	if !ok {
		return false
	}

	for _, c := range cs {
		if !check(c, arr, groups) {
			return false
		}
	}
	return true
}

func check(c Condition, arr value.ArrValue, groups SavedGroups) bool {
	for _, v := range arr {
		if c.Eval(v, groups) {
			return true
		}
	}
	return false
}
