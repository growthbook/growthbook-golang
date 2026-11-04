package condition

import (
	"cmp"
	"math"
	"unicode/utf16"
	"unicode/utf8"

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
		return compareUTF16(string(sa), string(b.(value.StrValue)))
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

// compareUTF16 orders strings by UTF-16 code units, like JS. This differs
// from byte (code point) order only when an astral character, a surrogate
// pair in UTF-16, meets a character in U+E000..U+FFFF.
func compareUTF16(a, b string) int {
	for a != "" && b != "" {
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		if ra != rb {
			if ua, ub := firstUnit(ra), firstUnit(rb); ua != ub {
				return cmp.Compare(ua, ub)
			}
			return cmp.Compare(ra, rb)
		}
		a, b = a[na:], b[nb:]
	}
	return cmp.Compare(len(a), len(b))
}

// firstUnit returns the first UTF-16 code unit of r.
func firstUnit(r rune) rune {
	if r >= 0x10000 {
		hi, _ := utf16.EncodeRune(r)
		return hi
	}
	return r
}
