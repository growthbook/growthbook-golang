package condition

import (
	"github.com/growthbook/growthbook-golang/internal/value"
)

// Condition evaluates a conditional expression. Every recursive evaluation must
// forward visited to retain the branch's cycle guard. Base.Eval starts a new
// evaluation with an empty path.
type Condition interface {
	Eval(value.Value, SavedGroups, visitedGroups) bool
}

// evalAny is an $or: an empty list passes, one passing condition decides it,
// and otherwise an unknown condition makes the result unknown.
func evalAny(cs []Condition, actual value.Value, groups SavedGroups, visited visitedGroups) result {
	if len(cs) == 0 {
		return pass
	}
	res := fail
	for _, c := range cs {
		switch eval3(c, actual, groups, visited) {
		case pass:
			return pass
		case unknown:
			res = unknown
		}
	}
	return res
}

// evalAll is an $and: one failing condition decides it, wherever an unknown
// one is.
func evalAll(cs []Condition, actual value.Value, groups SavedGroups, visited visitedGroups) result {
	res := pass
	for _, c := range cs {
		switch eval3(c, actual, groups, visited) {
		case fail:
			return fail
		case unknown:
			res = unknown
		}
	}
	return res
}
