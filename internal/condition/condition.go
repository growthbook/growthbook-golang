package condition

import (
	"github.com/growthbook/growthbook-golang/internal/value"
)

// Condition evaluates a conditional expression. Every recursive evaluation must
// forward visited to retain the branch's cycle guard. Base.Eval starts a new
// evaluation with an empty set.
type Condition interface {
	Eval(value.Value, SavedGroups, visitedGroups) bool
}

func evalAny(cs []Condition, actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	if len(cs) == 0 {
		return true
	}
	for _, c := range cs {
		if c.Eval(actual, groups, visited) {
			return true
		}
	}
	return false
}

func evalAll(cs []Condition, actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	for _, c := range cs {
		if !c.Eval(actual, groups, visited) {
			return false
		}
	}
	return true
}
