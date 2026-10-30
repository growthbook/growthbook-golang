package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// AllConds checks each condition is true for at least
// one array element.
type AllConds []Condition

func (cs AllConds) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return cs.eval3(actual, groups, visited) == pass
}

func (cs AllConds) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	return evalEach(cs, actual, groups, visited)
}

// AlliConds checks each condition is true for at least
// one array element (case-insensitive for strings).
type AlliConds []Condition

func (cs AlliConds) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return cs.eval3(actual, groups, visited) == pass
}

func (cs AlliConds) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	return evalEach(cs, actual, groups, visited)
}

// evalEach requires every condition to pass for at least one element. A
// condition that no element passes, but some element leaves unknown, makes the
// result unknown.
func evalEach(cs []Condition, actual value.Value, groups SavedGroups, visited visitedGroups) result {
	arr, ok := actual.(value.ArrValue)
	if !ok {
		return fail
	}
	res := pass
	for _, c := range cs {
		matched := fail
		for _, v := range arr {
			r := eval3(c, v, groups, visited)
			if r == pass {
				matched = pass
				break
			}
			if r == unknown {
				matched = unknown
			}
		}
		if matched == fail {
			return fail
		}
		if matched == unknown {
			res = unknown
		}
	}
	return res
}
