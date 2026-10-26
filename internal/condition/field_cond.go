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
	// A path into something that isn't an object is missing, as in JS getPath
	fieldValue := value.Null()
	if obj, ok := actual.(value.ObjValue); ok {
		fieldValue = obj.Path(c.path...)
	}
	return eval3(c.cond, fieldValue, groups, visited)
}

func NewFieldCond(pathStr string, cond Condition) FieldCond {
	path := strings.Split(pathStr, ".")
	return FieldCond{path, cond}
}
