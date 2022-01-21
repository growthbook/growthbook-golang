package growthbook

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
)

// Condition ...
type Condition interface {
	Eval(attrs Attributes) bool
}

type orCondition struct {
	conds []Condition
}

type norCondition struct {
	conds []Condition
}

type andCondition struct {
	conds []Condition
}

type notCondition struct {
	cond Condition
}

type operatorCondition struct {
	values map[string]interface{}
}

func isOperatorObject(obj map[string]interface{}) bool {
	for k := range obj {
		if !strings.HasPrefix(k, "$") {
			return false
		}
	}
	return true
}

func (cond orCondition) Eval(attrs Attributes) bool {
	if len(cond.conds) == 0 {
		return true
	}
	for i := range cond.conds {
		if cond.conds[i].Eval(attrs) {
			return true
		}
	}
	return false
}

func (cond norCondition) Eval(attrs Attributes) bool {
	or := orCondition{cond.conds}
	return !or.Eval(attrs)
}

func (cond notCondition) Eval(attrs Attributes) bool {
	return !cond.cond.Eval(attrs)
}

func (cond andCondition) Eval(attrs Attributes) bool {
	for i := range cond.conds {
		if !cond.conds[i].Eval(attrs) {
			return false
		}
	}
	return true
}

func getPath(attrs Attributes, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{}
	for i, p := range parts {
		if i == 0 {
			current = attrs[p]
		} else {
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil
			}
			current = m[p]
		}
	}
	return current
}

func getType(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "unknown"
	}
}

func compare(comp string, x interface{}, y interface{}) bool {
	switch x.(type) {
	case float64:
		xn := x.(float64)
		yn, ok := y.(float64)
		if !ok {
			// TODO: LOG ERROR HERE?
			return false
		}
		switch comp {
		case "$lt":
			return xn < yn
		case "$lte":
			return xn <= yn
		case "$gt":
			return xn > yn
		case "$gte":
			return xn >= yn
		}

	case string:
		xs := x.(string)
		ys, ok := y.(string)
		if !ok {
			// TODO: LOG ERROR HERE?
			return false
		}
		switch comp {
		case "$lt":
			return xs < ys
		case "$lte":
			return xs <= ys
		case "$gt":
			return xs > ys
		case "$gte":
			return xs >= ys
		}
	}
	return false
}

func elementIn(v interface{}, array interface{}) bool {
	vals, ok := array.([]interface{})
	if !ok {
		return false
	}
	for _, val := range vals {
		if reflect.DeepEqual(v, val) {
			return true
		}
	}
	return false
}

func elemMatch(attributeValue interface{}, conditionValue interface{}) bool {
	attrs, ok := attributeValue.([]interface{})
	if !ok {
		return false
	}
	condmap, ok := conditionValue.(map[string]interface{})
	if !ok {
		return false
	}
	check := func(v interface{}) bool { return evalConditionValue(conditionValue, v) }
	if !isOperatorObject(condmap) {
		cond, err := BuildCondition(condmap)
		if err != nil {
			return false
		}

		check = func(v interface{}) bool {
			vmap, ok := v.(map[string]interface{})
			if !ok {
				return false
			}
			as := Attributes(vmap)
			return cond.Eval(as)
		}
	}
	for _, a := range attrs {
		if check(a) {
			return true
		}
	}
	return false
}

func existsCheck(conditionValue interface{}, attributeValue interface{}) bool {
	cond, ok := conditionValue.(bool)
	if !ok {
		return false
	}
	if !cond {
		return attributeValue == nil
	}
	return attributeValue != nil
}

func evalAll(conditionValue interface{}, attributeValue interface{}) bool {
	conds, okc := conditionValue.([]interface{})
	attrs, oka := attributeValue.([]interface{})
	if !okc || !oka {
		return false
	}
	for _, c := range conds {
		passed := false
		for _, a := range attrs {
			if evalConditionValue(c, a) {
				passed = true
				break
			}
		}
		if !passed {
			return false
		}
	}
	return true
}

func evalOperatorCondition(key string, attributeValue interface{}, conditionValue interface{}) bool {
	// fmt.Printf("evalOperatorCondition: key=%s attr=%#v cond=%#v\n", key, attributeValue, conditionValue)
	switch key {
	case "$eq":
		return reflect.DeepEqual(attributeValue, conditionValue)

	case "$ne":
		return !reflect.DeepEqual(attributeValue, conditionValue)

	case "$lt", "$lte", "$gt", "$gte":
		return compare(key, attributeValue, conditionValue)

	case "$regex":
		restring, reok := conditionValue.(string)
		attrstring, attrok := attributeValue.(string)
		if !reok || !attrok {
			return false
		}
		re, err := regexp.Compile(restring)
		if err != nil {
			return false
		}
		return re.MatchString(attrstring)

	case "$in":
		return elementIn(attributeValue, conditionValue)

	case "$nin":
		return !elementIn(attributeValue, conditionValue)

	case "$elemMatch":
		return elemMatch(attributeValue, conditionValue)

	case "$size":
		if getType(attributeValue) != "array" {
			return false
		}
		return evalConditionValue(conditionValue, float64(len(attributeValue.([]interface{}))))

	case "$all":
		return evalAll(conditionValue, attributeValue)

	case "$exists":
		return existsCheck(conditionValue, attributeValue)

	case "$type":
		return getType(attributeValue) == conditionValue.(string)

	case "$not":
		return !evalConditionValue(conditionValue, attributeValue)

	default:
		return false
	}
}

func evalConditionValue(conditionValue interface{}, attributeValue interface{}) bool {
	// fmt.Printf("evalConditionValue: conditionValue=%#v  attributeValue=%#v\n", conditionValue, attributeValue)
	condmap, ok := conditionValue.(map[string]interface{})
	if ok && isOperatorObject(condmap) {
		for k, v := range condmap {
			if !evalOperatorCondition(k, attributeValue, v) {
				return false
			}
		}
		return true
	}

	return reflect.DeepEqual(conditionValue, attributeValue)
}

func (cond operatorCondition) Eval(attrs Attributes) bool {
	for k, v := range cond.values {
		if !evalConditionValue(v, getPath(attrs, k)) {
			return false
		}
	}
	return true
}

// ParseCondition ...
func ParseCondition(data []byte) (Condition, error) {
	topLevel := map[string]interface{}{}
	err := json.Unmarshal(data, &topLevel)
	if err != nil {
		return nil, err
	}

	return BuildCondition(topLevel)
}

// BuildCondition ...
func BuildCondition(cond map[string]interface{}) (Condition, error) {
	if or, ok := cond["$or"]; ok {
		conds, err := buildSeq(or)
		if err != nil {
			return nil, err
		}
		return orCondition{conds}, nil
	}

	if nor, ok := cond["$nor"]; ok {
		conds, err := buildSeq(nor)
		if err != nil {
			return nil, err
		}
		return norCondition{conds}, nil
	}

	if and, ok := cond["$and"]; ok {
		conds, err := buildSeq(and)
		if err != nil {
			return nil, err
		}
		return andCondition{conds}, nil
	}

	if not, ok := cond["$not"]; ok {
		subcond, ok := not.(map[string]interface{})
		if !ok {
			return nil, errors.New("something wrong in $not")
		}
		// fmt.Printf("===> subcond = %#v\n", subcond)
		cond, err := BuildCondition(subcond)
		if err != nil {
			return nil, err
		}
		return notCondition{cond}, nil
	}

	return operatorCondition{cond}, nil
}

func buildSeq(seq interface{}) ([]Condition, error) {
	conds, ok := seq.([]interface{})
	if !ok {
		return nil, errors.New("something wrong in condition sequence")
	}
	retval := make([]Condition, len(conds))
	for i := range conds {
		condmap, ok := conds[i].(map[string]interface{})
		if !ok {
			return nil, errors.New("something wrong in condition sequence element")
		}
		cond, err := BuildCondition(condmap)
		if err != nil {
			return nil, err
		}
		retval[i] = cond
	}
	return retval, nil
}
