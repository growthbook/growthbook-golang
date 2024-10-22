package condition

import (
	"github.com/growthbook/growthbook-golang/internal/value"
)

// Condition evaluates conditional expression
type Condition interface {
	Eval(obj value.ObjValue) bool
}

// Base is the top-level structure for MongoDB-like conditions
type Base []Condition

func (cs Base) Eval(obj value.ObjValue) bool {
	for _, c := range cs {
		if !c.Eval(obj) {
			return false
		}
	}
	return true
}
