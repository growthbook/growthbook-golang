package condition

import "github.com/growthbook/growthbook-golang/internal/value"

type AndConds []Condition

func (cs AndConds) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return evalAll(cs, actual, groups, visited)
}

type OrConds []Condition

func (conds OrConds) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return evalAny(conds, actual, groups, visited)
}

type NorConds []Condition

func (conds NorConds) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return !evalAny(conds, actual, groups, visited)
}

type NotCond struct {
	cond Condition
}

func (c NotCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return !c.cond.Eval(actual, groups, visited)
}
