package condition

import (
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

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

// equalCaseInsensitive performs case-insensitive equality comparison for strings,
// falls back to standard equality for other types
func equalCaseInsensitive(v1, v2 value.Value) bool {
	// If both are strings, use case-insensitive comparison
	if s1, ok1 := v1.(value.StrValue); ok1 {
		if s2, ok2 := v2.(value.StrValue); ok2 {
			return strings.EqualFold(string(s1), string(s2))
		}
	}
	// For non-strings, use standard equality
	return value.Equal(v1, v2)
}

// isInCaseInsensitive performs case-insensitive membership check for strings,
// falls back to standard equality for other types
func isInCaseInsensitive(fieldVal value.Value, expected value.ArrValue) bool {
	for _, ev := range expected {
		if equalCaseInsensitive(fieldVal, ev) {
			return true
		}
	}
	return false
}
