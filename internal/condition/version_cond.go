package condition

import (
	"regexp"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// VersionCond compares version numbers
type VersionCond struct {
	op      Operator
	version string
}

func NewVersionCond(op Operator, arg any) VersionCond {
	version := paddedVersionString(value.New(arg))
	return VersionCond{op, version}
}

func (c VersionCond) Eval(actual value.Value, _ SavedGroups) bool {
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
			val := strings.TrimLeft(p, "0") // remove leading zeros
			parts[i] = strings.Repeat(" ", 5-len(val)) + val
		}
	}
	return strings.Join(parts, "-")
}
