package condition

import "github.com/growthbook/growthbook-golang/internal/value"

type And []Base

func (c And) Eval(obj value.ObjValue) bool {
	return evalAnd(c, obj)
}

type Or []Base

func (c Or) Eval(obj value.ObjValue) bool {
	return evalOr(c, obj)
}

type Nor []Base

func (c Nor) Eval(obj value.ObjValue) bool {
	return !evalOr(c, obj)
}

type Not Base

func (c Not) Eval(obj value.ObjValue) bool {
	return !Base(c).Eval(obj)
}

func evalOr(cs []Base, obj value.ObjValue) bool {
	if len(cs) == 0 {
		return true
	}
	for _, c := range cs {
		if c.Eval(obj) {
			return true
		}
	}
	return false
}

func evalAnd(cs []Base, obj value.ObjValue) bool {
	for _, c := range cs {
		if !c.Eval(obj) {
			return false
		}
	}
	return true
}
