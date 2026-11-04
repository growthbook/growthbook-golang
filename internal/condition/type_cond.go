package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// TypeCond checks if value has proper type
type TypeCond struct {
	t     value.ValueType
	known bool
}

func NewTypeCond(arg string) TypeCond {
	t, known := typeFromName(arg)
	return TypeCond{t, known}
}

// typeFromName maps a JS type name to a value type. A missing attribute is
// null, so "undefined" (like any unknown name) never matches.
func typeFromName(arg string) (value.ValueType, bool) {
	switch arg {
	case "string":
		return value.StrType, true
	case "number":
		return value.NumType, true
	case "boolean":
		return value.BoolType, true
	case "object":
		return value.ObjType, true
	case "array":
		return value.ArrType, true
	case "null":
		return value.NullType, true
	default:
		return value.NullType, false
	}
}

func (c TypeCond) Eval(actual value.Value, _ SavedGroups, _ visitedGroups) bool {
	return c.known && actual.Type() == c.t
}
