package condition

import (
	"github.com/growthbook/growthbook-golang/internal/value"
)

// Condition evaluates conditional expression
type Condition interface {
	Eval(value.Value, SavedGroups) bool
}

func evalAny(cs []Condition, actual value.Value, groups SavedGroups) bool {
	if len(cs) == 0 {
		return true
	}
	for _, c := range cs {
		if c.Eval(actual, groups) {
			return true
		}
	}
	return false
}

func evalAll(cs []Condition, actual value.Value, groups SavedGroups) bool {
	for _, c := range cs {
		if !c.Eval(actual, groups) {
			return false
		}
	}
	return true
}
