package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// ValueCond used when field compared with another value directly, without any operator.
// Matches JS SDK semantics: primitive conditions coerce the attribute to the
// condition's type, except that boolean conditions never match null/missing.
type ValueCond struct {
	expected value.Value
}

func NewValueCond(arg any) ValueCond {
	return ValueCond{value.New(arg)}
}

func (c ValueCond) Eval(actual value.Value, _ SavedGroups) bool {
	return valueCompare(actual, c.expected)
}

// ValueCondCaseInsensitive is like ValueCond but uses case-insensitive comparison for strings
type ValueCondCaseInsensitive struct {
	expected value.Value
}

func NewValueCondCaseInsensitive(arg any) ValueCondCaseInsensitive {
	return ValueCondCaseInsensitive{value.New(arg)}
}

func (c ValueCondCaseInsensitive) Eval(actual value.Value, _ SavedGroups) bool {
	return equalCaseInsensitive(actual, c.expected)
}
