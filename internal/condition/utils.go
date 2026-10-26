package condition

import (
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// valueCompare mirrors the JS SDK's direct comparison (mongrule.ts
// evalConditionValue): values match only when they have the same type and
// are equal, with arrays/objects compared by deep equality. There is no type
// coercion, and a null condition matches only null/missing.
func valueCompare(actual, expected value.Value) bool {
	return value.Equal(actual, expected)
}

// isOrderable reports whether a and b can be ordered: both numbers or both strings.
func isOrderable(a, b value.Value) bool {
	t := a.Type()
	return (t == value.NumType || t == value.StrType) && t == b.Type()
}

// strictEqual mirrors JS ===, used by $eq and $ne: an object or array never
// equals one parsed separately, and NaN equals nothing.
func strictEqual(a, b value.Value) bool {
	switch a.Type() {
	case value.ObjType, value.ArrType:
		return false
	}
	return value.Equal(a, b)
}

// sameValueZero mirrors JS Array.prototype.includes, used by $in and $nin: it
// is strictEqual except that NaN matches NaN.
func sameValueZero(a, b value.Value) bool {
	return strictEqual(a, b) || (isNaN(a) && isNaN(b))
}

func isIn(fieldVal value.Value, expected value.ArrValue) bool {
	for _, ev := range expected {
		if sameValueZero(fieldVal, ev) {
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
// and compares other values like JS === (see strictEqual)
func isInCaseInsensitive(fieldVal value.Value, expected value.ArrValue) bool {
	for _, ev := range expected {
		if s1, ok := fieldVal.(value.StrValue); ok {
			if s2, ok := ev.(value.StrValue); ok && strings.EqualFold(string(s1), string(s2)) {
				return true
			}
			continue
		}
		if strictEqual(fieldVal, ev) {
			return true
		}
	}
	return false
}
