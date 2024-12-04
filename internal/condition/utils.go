package condition

import "github.com/growthbook/growthbook-golang/internal/value"

func valueCompare(actual, expected value.Value) bool {
	switch expected.Type() {
	case value.StrType, value.NumType, value.BoolType:
		casted := actual.Cast(expected.Type())
		return value.Equal(expected, casted)
	case value.NullType:
		return value.IsNull(actual)
	default:
		return value.Equal(actual, expected)
	}
}

func isIn(fieldVal value.Value, expected value.ArrValue) bool {
	for _, ev := range expected {
		if value.Equal(fieldVal, ev) {
			return true
		}
	}
	return false
}
