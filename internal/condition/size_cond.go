package condition

import "github.com/growthbook/growthbook-golang/internal/value"

// SizeCond checks length of field array
type SizeCond struct {
	expected value.Value
}

func NewSizeCond(arg any) SizeCond {
	return SizeCond{value.New(arg)}
}

func (c SizeCond) Eval(actual value.Value, _ SavedGroups) bool {
	if arr, ok := actual.(value.ArrValue); ok {
		return valueCompare(c.expected, value.New(len(arr)))
	}
	return false
}
