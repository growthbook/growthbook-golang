package condition

import (
	"github.com/growthbook/growthbook-golang/internal/value"
)

// ExistsCond checks if field value exists or not
type ExistsCond struct {
	expected bool
}

func NewExistsCond(arg any) ExistsCond {
	v := value.New(arg).Cast(value.BoolType)
	expected := value.Equal(v, value.True())
	return ExistsCond{expected}
}

func (op ExistsCond) Eval(actual value.Value, _ SavedGroups) bool {
	if op.expected {
		return !value.IsNull(actual)
	} else {
		return value.IsNull(actual)
	}
}
