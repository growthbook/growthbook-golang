package condition

import (
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// VersionCond compares version numbers
type VersionCond struct {
	op      Operator
	arg     value.Value
	version string
}

func NewVersionCond(op Operator, arg any) VersionCond {
	v := value.New(arg)
	return VersionCond{op, v, paddedVersionString(v)}
}

func (c VersionCond) Eval(actual value.Value, _ SavedGroups, _ visitedGroups) bool {
	// Both sides must be strings, or both numbers. On a type mismatch every
	// operator fails except $vne, the negation of $veq.
	if !isOrderable(actual, c.arg) {
		return c.op == vneOp
	}
	switch c.op {
	case veqOp:
		return paddedVersionString(actual) == c.version
	case vneOp:
		return paddedVersionString(actual) != c.version
	case vgtOp:
		return paddedVersionString(actual) > c.version
	case vgteOp:
		return paddedVersionString(actual) >= c.version
	case vltOp:
		return paddedVersionString(actual) < c.version
	case vlteOp:
		return paddedVersionString(actual) <= c.version
	}
	return false
}

// paddedVersionString mirrors the JS SDK's paddedVersionString: drop a
// leading "v" and any build metadata after the first "+", split on "-" and
// ".", add "~" to a plain x.y.z so it sorts after its pre-releases, and
// left-pad numeric parts to 5 characters so strings compare like numbers.
func paddedVersionString(input value.Value) string {
	var version string
	switch v := input.(type) {
	case value.NumValue, value.StrValue:
		version = v.String()
	}
	if version == "" {
		version = "0"
	}
	version = strings.TrimPrefix(version, "v")
	if i := strings.IndexByte(version, '+'); i >= 0 {
		version = version[:i]
	}

	var b strings.Builder
	b.Grow(len(version) + 24)
	parts := 0
	writePart := func(p string) {
		if parts > 0 {
			b.WriteByte('-')
		}
		parts++
		if len(p) > 0 && len(p) < 5 && isDigits(p) {
			for i := len(p); i < 5; i++ {
				b.WriteByte(' ')
			}
		}
		b.WriteString(p)
	}
	start := 0
	for i := 0; i < len(version); i++ {
		if version[i] == '-' || version[i] == '.' {
			writePart(version[start:i])
			start = i + 1
		}
	}
	writePart(version[start:])
	if parts == 3 {
		writePart("~")
	}
	return b.String()
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
