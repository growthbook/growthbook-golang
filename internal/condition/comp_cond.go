package condition

import (
	"math"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// CompCond compares values with $eq, $ne, $lt, $lte, $gt and $gte
type CompCond struct {
	op  Operator
	arg value.Value
}

func NewCompCond(op Operator, arg any) CompCond {
	return CompCond{op, value.New(arg)}
}

func (c CompCond) Eval(actual value.Value, _ SavedGroups, _ visitedGroups) bool {
	switch c.op {
	case eqOp:
		return strictEqual(c.arg, actual)
	case neOp:
		return !strictEqual(c.arg, actual)
	}
	// Only numbers with numbers and strings with strings are ordered;
	// any other pair fails, including booleans and null/missing values.
	if !isOrderable(actual, c.arg) {
		return false
	}
	// NaN is unordered, so every ordering with it fails, as in JS
	if isNaN(actual) || isNaN(c.arg) {
		return false
	}
	cmp := compare(actual, c.arg)
	switch c.op {
	case ltOp:
		return cmp < 0
	case lteOp:
		return cmp <= 0
	case gtOp:
		return cmp > 0
	case gteOp:
		return cmp >= 0
	}
	return false
}

// compare orders two values of the same orderable type (see isOrderable).
// Strings compare as text, so "10" < "9".
func compare(a, b value.Value) int {
	if sa, ok := a.(value.StrValue); ok {
		return strings.Compare(string(sa), string(b.(value.StrValue)))
	}
	na, nb := a.(value.NumValue), b.(value.NumValue)
	switch {
	case na < nb:
		return -1
	case na > nb:
		return 1
	default:
		return 0
	}
}

func isNaN(v value.Value) bool {
	n, ok := v.(value.NumValue)
	return ok && math.IsNaN(float64(n))
}
