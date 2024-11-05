package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// TypeCond checks if value has proper type
type TypeCond struct {
	t value.ValueType
}

func NewTypeCond(arg string) TypeCond {
	return TypeCond{typeFromName(arg)}
}

func typeFromName(arg string) value.ValueType {
	switch arg {
	case "string":
		return value.StrType
	case "number":
		return value.NumType
	case "boolean":
		return value.BoolType
	case "object":
		return value.ObjType
	case "array":
		return value.ArrType
	default:
		return value.NullType
	}
}

func (c TypeCond) Eval(actual value.Value, _ SavedGroups) bool {
	return actual.Type() == c.t
}
