package condition

import (
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

type FieldCond struct {
	path []string
	cond Condition
}

func (c FieldCond) Eval(actual value.Value, groups SavedGroups, visited visitedGroups) bool {
	return c.eval3(actual, groups, visited) == pass
}

func (c FieldCond) eval3(actual value.Value, groups SavedGroups, visited visitedGroups) result {
	// As in JS getPath, an array's own keys are its indexes and length, so a
	// condition inside $elemMatch can test a nested array element
	return eval3(c.cond, value.Path(actual, c.path...), groups, visited)
}

func NewFieldCond(pathStr string, cond Condition) FieldCond {
	path := strings.Split(pathStr, ".")
	return FieldCond{path, cond}
}
