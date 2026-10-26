package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// result is the outcome of a condition: it passes, fails, or depends on an
// operator or saved group type this SDK doesn't support. Only pass counts as a
// match, so an unsupported condition fails even under $not or $nor.
type result int8

const (
	fail result = iota
	pass
	unknown
)

func resultOf(b bool) result {
	if b {
		return pass
	}
	return fail
}

func (r result) not() result {
	switch r {
	case pass:
		return fail
	case fail:
		return pass
	}
	return unknown
}

// triCondition is implemented by conditions whose result can be unknown, or
// that pass on an unknown result from a nested condition.
type triCondition interface {
	eval3(value.Value, SavedGroups, visitedGroups) result
}

func eval3(c Condition, actual value.Value, groups SavedGroups, visited visitedGroups) result {
	if t, ok := c.(triCondition); ok {
		return t.eval3(actual, groups, visited)
	}
	return resultOf(c.Eval(actual, groups, visited))
}

// unknownOpCond stands in for an operator this SDK doesn't support. The client
// evaluating the payload may support it, so the result is unknown.
type unknownOpCond struct{}

func (c unknownOpCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return c.eval3(actual, groups, visited) == pass
}

func (unknownOpCond) eval3(value.Value, SavedGroups, visitedGroups) result {
	return unknown
}
