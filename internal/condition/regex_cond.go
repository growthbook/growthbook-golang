package condition

import (
	"regexp"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// RegexCond implements regex comparison
type RegexCond struct {
	rx *regexp.Regexp
}

func NewRegexCond(rx *regexp.Regexp) RegexCond {
	return RegexCond{rx}
}

func (c RegexCond) Eval(actual value.Value, _ SavedGroups) bool {
	if s, ok := actual.(value.StrValue); ok {
		return c.rx.MatchString(string(s))
	}
	return false
}
