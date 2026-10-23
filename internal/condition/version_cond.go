package condition

import (
	"regexp"
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

var (
	replaceRe      = regexp.MustCompile(`(^v|\+.*$)`)
	versionSplitRe = regexp.MustCompile(`[-.]`)
	versionNumRe   = regexp.MustCompile(`^[0-9]+$`)
)

func paddedVersionString(input value.Value) string {
	var version string
	switch v := input.(type) {
	case value.NumValue, value.StrValue:
		version = v.String()
	}
	if version == "" {
		version = "0"
	}
	version = replaceRe.ReplaceAllString(version, "")
	parts := versionSplitRe.Split(version, -1)
	if len(parts) == 3 {
		parts = append(parts, "~")
	}
	for i, p := range parts {
		isNumber := versionNumRe.MatchString(p)
		if isNumber && len(p) < 5 {
			parts[i] = strings.Repeat(" ", 5-len(p)) + p
		}
	}
	return strings.Join(parts, "-")
}
