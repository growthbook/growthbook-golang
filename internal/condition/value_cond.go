package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// ValueCond used when field compared with another value directly, without any operator
// Growthbook implementation casts field value to expected type in that case before comparison.
type ValueCond struct {
	expected value.Value
}

func NewValueCond(arg any) ValueCond {
	return ValueCond{value.New(arg)}
}

func (c ValueCond) Eval(actual value.Value, _ SavedGroups) bool {
	return valueCompare(actual, c.expected)
}
