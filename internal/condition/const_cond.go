package condition

import "github.com/growthbook/growthbook-golang/internal/value"

type True struct{}
type False struct{}

func (True) Eval(value.Value, SavedGroups, visitedGroups) bool {
	return true
}

func (False) Eval(value.Value, SavedGroups, visitedGroups) bool {
	return false
}
