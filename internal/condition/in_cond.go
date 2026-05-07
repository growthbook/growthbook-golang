package condition

import (
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// InOp checks if value is in array
type InCond struct {
	expected value.ArrValue
	strSet   map[string]struct{}
}

func NewInCond(arg value.ArrValue) InCond {
	return InCond{expected: arg, strSet: buildStrSet(arg)}
}

func NewNotInCond(arg value.ArrValue) Condition {
	cond := NewInCond(arg)
	return NotCond{cond}
}

func (c InCond) Eval(actual value.Value, _ SavedGroups) bool {
	if arr, ok := actual.(value.ArrValue); ok {
		for _, v := range arr {
			if c.contains(v) {
				return true
			}
		}
		return false
	}
	return c.contains(actual)
}

func (c InCond) contains(v value.Value) bool {
	if c.strSet != nil {
		if s, ok := v.(value.StrValue); ok {
			_, hit := c.strSet[string(s)]
			return hit
		}
	}
	return isIn(v, c.expected)
}

// IniCond checks if value is in array (case-insensitive for strings)
type IniCond struct {
	expected value.ArrValue
	strSet   map[string]struct{}
}

func NewIniCond(arg value.ArrValue) IniCond {
	return IniCond{expected: arg, strSet: buildLowerASCIIStringSet(arg)}
}

func NewNotIniCond(arg value.ArrValue) Condition {
	cond := NewIniCond(arg)
	return NotCond{cond}
}

func (c IniCond) Eval(actual value.Value, _ SavedGroups) bool {
	if arr, ok := actual.(value.ArrValue); ok {
		for _, v := range arr {
			if c.contains(v) {
				return true
			}
		}
		return false
	}
	return c.contains(actual)
}

func (c IniCond) contains(v value.Value) bool {
	if c.strSet != nil {
		if s, ok := v.(value.StrValue); ok {
			actual := string(s)
			if isASCII(actual) {
				_, hit := c.strSet[strings.ToLower(actual)]
				return hit
			}
		}
	}
	return isInCaseInsensitive(v, c.expected)
}

// buildStrSet returns a string set when every element of arg is a StrValue,
// otherwise nil so callers fall back to the linear scan.
func buildStrSet(arg value.ArrValue) map[string]struct{} {
	if len(arg) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(arg))
	for _, v := range arg {
		s, ok := v.(value.StrValue)
		if !ok {
			return nil
		}
		set[string(s)] = struct{}{}
	}
	return set
}

// buildLowerASCIIStringSet is the $ini fast path. It only indexes ASCII
// strings so the slow path keeps the exact EqualFold behavior for Unicode.
func buildLowerASCIIStringSet(arg value.ArrValue) map[string]struct{} {
	if len(arg) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(arg))
	for _, v := range arg {
		s, ok := v.(value.StrValue)
		if !ok {
			return nil
		}
		k := string(s)
		if !isASCII(k) {
			return nil
		}
		set[strings.ToLower(k)] = struct{}{}
	}
	return set
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}
