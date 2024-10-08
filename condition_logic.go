package growthbook

import (
	"encoding/json"
	"errors"
)

type logicOp string

const (
	andOp logicOp = "$and"
	orOp  logicOp = "$or"
	norOp logicOp = "$nor"
	notOp logicOp = "$not"
)

var (
	errCondLogicalValueIsNotArray = errors.New("Condition's logical value is not array")
	errCondUnknownLogicOperator   = errors.New("Unknown condition logic operator")
)

type condLogic struct {
	op    logicOp
	conds []condEval
}

func (cond *condLogic) UnmarshalJSON(data []byte) error {
	switch cond.op {
	case andOp, orOp, norOp:
		newConds := []*Condition{}
		err := json.Unmarshal(data, newConds)
		if err != nil {
			return err
		}
		for _, newCond := range newConds {
			cond.conds = append(cond.conds, newCond)
		}

		return nil
	case notOp:
		newCond := &Condition{}
		err := json.Unmarshal(data, newCond)
		if err != nil {
			return err
		}
		cond.conds = append(cond.conds, &Condition{cond: newCond})
		return nil
	default:
		return errCondUnknownLogicOperator
	}

}

func (cond *condLogic) eval(c *Client, attributes Attributes) bool {
	switch cond.op {
	case andOp:
		return evalAnd(c, cond.conds, attributes)
	case orOp:
		return evalOr(c, cond.conds, attributes)
	case norOp:
		return !evalOr(c, cond.conds, attributes)
	case notOp:
		return !evalAnd(c, cond.conds, attributes)
	default:
		c.logger.Error("Condition: invalid logic op", "op", string(cond.op))
		return false
	}
}

func evalAnd(c *Client, conds []condEval, attributes Attributes) bool {
	for _, cond := range conds {
		if !cond.eval(c, attributes) {
			return false
		}
	}
	return true
}

func evalOr(c *Client, conds []condEval, attributes Attributes) bool {
	if len(conds) == 0 {
		return true
	}
	for _, cond := range conds {
		if cond.eval(c, attributes) {
			return true
		}
	}
	return false
}

func condIsLogicOp(k string) bool {
	switch logicOp(k) {
	case andOp, orOp, norOp, notOp:
		return true
	default:
		return false
	}
}
