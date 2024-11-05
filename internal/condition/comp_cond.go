package condition

import (
	"github.com/growthbook/growthbook-golang/internal/value"
)

// CompCond compares values using JS comparison
type CompCond struct {
	op  Operator
	arg value.Value
}

func NewCompCond(op Operator, arg any) CompCond {
	return CompCond{op, value.New(arg)}
}

func (c CompCond) Eval(actual value.Value, _ SavedGroups) bool {
	switch c.op {
	case eqOp:
		return value.Equal(c.arg, actual)
	case neOp:
		return !value.Equal(c.arg, actual)
	}
	cmp := jsCompare(actual, c.arg)
	switch c.op {
	case ltOp:
		return cmp == -1
	case lteOp:
		return cmp == -1 || cmp == 0
	case gtOp:
		return cmp == 1
	case gteOp:
		return cmp == 1 || cmp == 0
	}
	return false
}

// JsCompare implements JS comparison algorithm, returns:
//   - 0, a ==b
//   - 1, a > b
//   - -1, a < b
//   - 2, a and b are not comparable
func jsCompare(a, b value.Value) int {
	if value.IsNull(a) && value.IsNull(b) {
		return 0
	}
	sa, oka := a.(value.StrValue)
	sb, okb := b.(value.StrValue)
	if oka && okb {
		switch {
		case sa < sb:
			return -1
		case sa == sb:
			return 0
		default:
			return 1
		}
	}
	a, b = a.Cast(value.NumType), b.Cast(value.NumType)
	na, oka := a.(value.NumValue)
	nb, okb := b.(value.NumValue)
	if oka && okb {
		switch {
		case na < nb:
			return -1
		case na == nb:
			return 0
		default:
			return 1
		}
	}
	return 2
}
