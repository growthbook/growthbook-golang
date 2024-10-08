package growthbook

import (
	"encoding/json"
)

type condEval interface {
	eval(c *Client, attributes Attributes) bool
}

// Condition is top-level structure for MongoDB-like query
type Condition struct {
	cond condEval
}

func (cond *Condition) eval(c *Client, attributes Attributes) bool {
	return cond.cond.eval(c, attributes)
}

func (c *Condition) UnmarshalJSON(data []byte) error {
	var m map[string]json.RawMessage
	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	cond, err := buildCond(m)
	if err != nil {
		return err
	}
	c.cond = cond
	return nil
}

func buildCond(ms map[string]json.RawMessage) (condEval, error) {
	conds := []condEval{}
	for k, m := range ms {
		if condIsLogicOp(k) {
			cond := &condLogic{
				op: logicOp(k),
			}
			err := json.Unmarshal(m, cond)
			if err != nil {
				return nil, err
			}
			conds = append(conds, cond)
		} else {
			cond := &condField{
				path: k,
			}
			err := json.Unmarshal(m, cond)
			if err != nil {
				return nil, err
			}
			conds = append(conds, cond)
		}
	}
	switch len(conds) {
	case 1:
		return conds[0], nil
	default:
		return &condLogic{op: andOp, conds: nil}, nil
	}
}
