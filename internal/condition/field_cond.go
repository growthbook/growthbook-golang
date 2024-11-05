package condition

import (
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

type FieldCond struct {
	path []string
	cond Condition
}

func (c FieldCond) Eval(actual value.Value, groups SavedGroups) bool {
	obj, ok := actual.(value.ObjValue)
	if !ok {
		return false
	}
	fieldValue := obj.Path(c.path...)
	return c.cond.Eval(fieldValue, groups)
}

func NewFieldCond(pathStr string, cond Condition) FieldCond {
	path := strings.Split(pathStr, ".")
	return FieldCond{path, cond}
}
