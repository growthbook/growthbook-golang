package condition

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/growthbook/growthbook-golang/internal/value"
)

type Base struct {
	cond Condition
}

func (base Base) Eval(actual value.Value, groups SavedGroups) bool {
	if base.cond == nil {
		return true
	}
	return base.cond.Eval(actual, groups)
}

func (base *Base) UnmarshalJSON(data []byte) error {
	m := map[string]any{}
	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	json := value.New(m)
	cond, err := buildBaseCond(json)
	if err != nil {
		return err
	}
	*base = Base{cond}
	return nil
}

func buildBaseCond(json value.Value) (Condition, error) {
	obj, ok := json.(value.ObjValue)
	if !ok {
		return nil, fmt.Errorf("Base expected to be an object")
	}
	conds := []Condition{}
	for f, fv := range obj {
		cond, err := buildLogicCond(f, fv)
		if err != nil {
			return Base{}, fmt.Errorf("Error building %v : %v", f, err)
		}
		conds = append(conds, cond)
	}
	if len(conds) == 1 {
		return conds[0], nil
	}
	return AndConds(conds), nil
}

func buildLogicCond(op string, arg value.Value) (Condition, error) {
	switch Operator(op) {
	case andOp, orOp, norOp:
		conds, err := buildBaseList(arg)
		if err != nil {
			return nil, fmt.Errorf("Error parsing `%v` condition: %w", op, err)
		}
		return newLogicCond(op, conds), nil
	case notOp:
		cond, err := buildBaseCond(arg)
		if err != nil {
			return nil, fmt.Errorf("Error parsing `%v` condition: %w", op, err)
		}
		return NotCond{cond}, nil
	default:
		return buildFieldCond(op, arg)
	}
}

func newLogicCond(op string, conds []Condition) Condition {
	switch Operator(op) {
	case andOp:
		return AndConds(conds)
	case orOp:
		return OrConds(conds)
	case norOp:
		return NorConds(conds)
	}
	return nil
}

func buildBaseList(json value.Value) ([]Condition, error) {
	arr, ok := json.(value.ArrValue)
	if !ok {
		return nil, fmt.Errorf("Array expected")
	}
	var res []Condition
	for _, v := range arr {
		b, err := buildBaseCond(v)
		if err != nil {
			return nil, err
		}
		res = append(res, b)
	}
	return res, nil
}

func buildFieldCond(path string, json value.Value) (Condition, error) {
	cond, err := buildValueCond(json)
	if err != nil {
		return nil, fmt.Errorf("Error parsing %v. %w", path, err)
	}
	return NewFieldCond(path, cond), nil
}

func buildValueCond(json value.Value) (Condition, error) {
	obj, ok := json.(value.ObjValue)
	if !(ok && isOperatorObject(obj)) {
		return NewValueCond(json), nil
	}

	return buildObjCond(obj)
}

func buildObjCond(obj value.ObjValue) (Condition, error) {
	var conds []Condition
	for op, arg := range obj {
		cond, err := buildOpCond(Operator(op), arg)
		if err != nil {
			return nil, err
		}
		conds = append(conds, cond)
	}
	if len(conds) == 1 {
		return conds[0], nil
	}
	return AndConds(conds), nil
}

func buildOpCond(op Operator, arg value.Value) (Condition, error) {
	switch op {
	case eqOp, neOp, ltOp, lteOp, gtOp, gteOp:
		return NewCompCond(op, arg), nil
	case veqOp, vneOp, vgtOp, vgteOp, vltOp, vlteOp:
		return NewVersionCond(op, arg), nil
	case inOp:
		arr, ok := arg.(value.ArrValue)
		if !ok {
			return False{}, nil
		}
		return NewInCond(arr), nil
	case ninOp:
		arr, ok := arg.(value.ArrValue)
		if !ok {
			return False{}, nil
		}
		return NewNotInCond(arr), nil
	case inGroupOp:
		str, ok := arg.(value.StrValue)
		if !ok {
			return nil, fmt.Errorf("$inGroup argument %v isn't a string", arg)
		}
		return NewInGroupCond(string(str)), nil
	case notInGroupOp:
		str, ok := arg.(value.StrValue)
		if !ok {
			return nil, fmt.Errorf("$notInGroup argument %v isn't a string", arg)
		}
		return NewNotInGroupCond(string(str)), nil
	case regexOp:
		return buildRegexCond(arg)
	case sizeOp:
		cond, err := buildValueCond(arg)
		if err != nil {
			return nil, fmt.Errorf("Error parsing $size operator: %w", err)
		}
		return NewSizeCond(cond), nil
	case typeOp:
		s, ok := arg.(value.StrValue)
		if !ok {
			return nil, fmt.Errorf("TypeOp argument %v isn't a string", arg)
		}
		return NewTypeCond(string(s)), nil
	case existsOp:
		return NewExistsCond(arg), nil
	case elemMatchOp:
		return buildElemMatchCond(arg)
	case allOp:
		return buildAllConds(arg)
	case notOp:
		cond, err := buildValueCond(arg)
		if err != nil {
			return nil, fmt.Errorf("Error parsing $not operator: %w", err)
		}
		return NotCond{cond}, nil
	default:
		return False{}, nil
	}
}

func isOperatorObject(obj value.ObjValue) bool {
	for k := range obj {
		if len(k) == 0 || k[0] != '$' {
			return false
		}
	}
	return true
}

func buildRegexCond(arg value.Value) (Condition, error) {
	s, ok := arg.(value.StrValue)
	if !ok {
		return nil, fmt.Errorf("RegexOp argument %v isn't a string", arg)
	}

	r, err := regexp.Compile(string(s))
	if err != nil {
		return False{}, nil
	}
	return NewRegexCond(r), nil
}

func buildElemMatchCond(arg value.Value) (Condition, error) {
	obj, ok := arg.(value.ObjValue)
	if !ok {
		return nil, fmt.Errorf("ElemMatch arg is not an object: %v", arg)
	}
	if isOperatorObject(obj) {
		cond, err := buildObjCond(obj)
		if err != nil {
			return nil, fmt.Errorf("ElemMatch arg %v is invalid: %w", arg, err)
		}
		return NewElemMatchCond(cond), nil
	}
	cond, err := buildBaseCond(obj)
	if err != nil {
		return nil, fmt.Errorf("ElemMatch arg %v is invalid: %w", arg, err)
	}
	return NewElemMatchCond(cond), nil
}

func buildAllConds(arg value.Value) (Condition, error) {
	arr, ok := arg.(value.ArrValue)
	if !ok {
		return nil, fmt.Errorf("$all arg %v is not an array", arg)
	}
	res := AllConds{}
	for _, v := range arr {
		c, err := buildValueCond(v)
		if err != nil {
			return nil, fmt.Errorf("$all arg is invalid: %w", err)
		}
		res = append(res, c)
	}
	return res, nil
}
