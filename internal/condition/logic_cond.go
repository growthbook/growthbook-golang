package condition

import "github.com/growthbook/growthbook-golang/internal/value"

type AndConds []Condition

func (cs AndConds) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return cs.eval3(actual, groups, visited) == pass
}

func (cs AndConds) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	return evalAll(cs, actual, groups, visited)
}

type OrConds []Condition

func (conds OrConds) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return conds.eval3(actual, groups, visited) == pass
}

func (conds OrConds) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	return evalAny(conds, actual, groups, visited)
}

type NorConds []Condition

func (conds NorConds) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return conds.eval3(actual, groups, visited) == pass
}

func (conds NorConds) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	return evalAny(conds, actual, groups, visited).not()
}

type NotCond struct {
	cond Condition
}

func (c NotCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return c.eval3(actual, groups, visited) == pass
}

func (c NotCond) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	return eval3(c.cond, actual, groups, visited).not()
}
