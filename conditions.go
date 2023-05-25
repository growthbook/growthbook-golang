package growthbook

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
)

// Condition represents conditions used to target features/experiments
// to specific users.
type Condition interface {
	Eval(attrs Attributes) bool
}

// Concrete condition representing ORing together a list of
// conditions.
type orCondition struct {
	conds []Condition
}

// Concrete condition representing NORing together a list of
// conditions.
type norCondition struct {
	conds []Condition
}

// Concrete condition representing ANDing together a list of
// conditions.
type andCondition struct {
	conds []Condition
}

// Concrete condition representing the complement of another
// condition.
type notCondition struct {
	cond Condition
}

// Concrete condition representing the base condition case of a set of
// keys and values or subsidiary conditions.
type baseCondition struct {
	// This is represented in this dynamically typed form to make lax
	// error handling easier.
	values map[string]interface{}
}

// Evaluate ORed list of conditions.
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

// Evaluate NORed list of conditions.
func (cond norCondition) Eval(attrs Attributes) bool {
	or := orCondition{cond.conds}
	return !or.Eval(attrs)
}

// Evaluate ANDed list of conditions.
func (cond andCondition) Eval(attrs Attributes) bool {
	for i := range cond.conds {
		if !cond.conds[i].Eval(attrs) {
			return false
		}
	}
	return true
}

// Evaluate complemented condition.
func (cond notCondition) Eval(attrs Attributes) bool {
	return !cond.cond.Eval(attrs)
}

// Evaluate base Condition case by iterating over keys and performing
// evaluation for each one (either a simple comparison, or an operator
// evaluation).
func (cond baseCondition) Eval(attrs Attributes) bool {
	for k, v := range cond.values {
		if !evalConditionValue(v, getPath(attrs, k)) {
			return false
		}
	}
	return true
}

// ParseCondition creates a Condition value from raw JSON input.
func ParseCondition(data []byte) Condition {
	topLevel := make(map[string]interface{})
	err := json.Unmarshal(data, &topLevel)
	if err != nil {
		logError("Failed parsing JSON input", "Condition")
		return nil
	}

	return BuildCondition(topLevel)
}

// BuildCondition creates a Condition value from a JSON object
// represented as a Go map.
func BuildCondition(cond map[string]interface{}) Condition {
	if or, ok := cond["$or"]; ok {
		conds := buildSeq(or)
		if conds == nil {
			return nil
		}
		return orCondition{conds}
	}

	if nor, ok := cond["$nor"]; ok {
		conds := buildSeq(nor)
		if conds == nil {
			return nil
		}
		return norCondition{conds}
	}

	if and, ok := cond["$and"]; ok {
		conds := buildSeq(and)
		if conds == nil {
			return nil
		}
		return andCondition{conds}
	}

	if not, ok := cond["$not"]; ok {
		subcond, ok := not.(map[string]interface{})
		if !ok {
			logError("Invalid $not in JSON condition data")
			return nil
		}
		cond := BuildCondition(subcond)
		if cond == nil {
			return nil
		}
		return notCondition{cond}
	}

	return baseCondition{cond}
}

//-- PRIVATE FUNCTIONS START HERE ----------------------------------------------

// Extract sub-elements of an attribute object using dot-separated
// paths.
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

// Process a sequence of JSON values into an array of Conditions.
func buildSeq(seq interface{}) []Condition {
	// The input should be a JSON array.
	conds, ok := seq.([]interface{})
	if !ok {
		logError("Something wrong in condition sequence")
		return nil
	}

	retval := make([]Condition, len(conds))
	for i := range conds {
		// Each condition in the sequence should be a JSON object.
		condmap, ok := conds[i].(map[string]interface{})
		if !ok {
			logError("Something wrong in condition sequence element")
		}
		cond := BuildCondition(condmap)
		if cond == nil {
			return nil
		}
		retval[i] = cond
	}
	return retval
}

// Evaluate one element of a base condition. If the condition value is
// a JSON object and each key in it is an operator name (e.g. "$eq",
// "$gt", "$elemMatch", etc.), then evaluate as an operator condition.
// Otherwise, just directly compare the condition value with the
// attribute value.
func evalConditionValue(condVal interface{}, attrVal interface{}) bool {
	condmap, ok := condVal.(map[string]interface{})
	if ok && isOperatorObject(condmap) {
		for k, v := range condmap {
			if !evalOperatorCondition(k, attrVal, v) {
				return false
			}
		}
		return true
	}

	return reflect.DeepEqual(condVal, attrVal)
}

// An operator object is a JSON object all of whose keys start with a
// "$" character, representing comparison operators.
func isOperatorObject(obj map[string]interface{}) bool {
	for k := range obj {
		if !strings.HasPrefix(k, "$") {
			return false
		}
	}
	return true
}

// Evaluate operator conditions. The first parameter here is the
// operator name.
func evalOperatorCondition(key string, attrVal interface{}, condVal interface{}) bool {
	switch key {
	case "$eq":
		return reflect.DeepEqual(attrVal, condVal)

	case "$ne":
		return !reflect.DeepEqual(attrVal, condVal)

	case "$lt", "$lte", "$gt", "$gte":
		return compare(key, attrVal, condVal)

	case "$regex":
		restring, reok := condVal.(string)
		attrstring, attrok := attrVal.(string)
		if !reok || !attrok {
			return false
		}
		re, err := regexp.Compile(restring)
		if err != nil {
			return false
		}
		return re.MatchString(attrstring)

	case "$in":
		return elementIn(attrVal, condVal)

	case "$nin":
		return !elementIn(attrVal, condVal)

	case "$elemMatch":
		return elemMatch(attrVal, condVal)

	case "$size":
		if getType(attrVal) != "array" {
			return false
		}
		return evalConditionValue(condVal, float64(len(attrVal.([]interface{}))))

	case "$all":
		return evalAll(condVal, attrVal)

	case "$exists":
		return existsCheck(condVal, attrVal)

	case "$type":
		return getType(attrVal) == condVal.(string)

	case "$not":
		return !evalConditionValue(condVal, attrVal)

	default:
		return false
	}
}

// Get JSON type name for Go representation of JSON objects.
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

// Perform numeric or string ordering comparisons on polymorphic JSON
// values.
func compare(comp string, x interface{}, y interface{}) bool {
	switch x.(type) {
	case float64:
		xn := x.(float64)
		yn, ok := y.(float64)
		if !ok {
			logWarn("Types don't match in condition comparison operation")
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
			logWarn("Types don't match in condition comparison operation")
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

// Check for membership of a JSON value in a JSON array.
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

// Perform "element matching" operation.
func elemMatch(attrVal interface{}, condVal interface{}) bool {
	// Check that the attribute and condition values are of the
	// appropriate types (an array and an object respectively).
	attrs, ok := attrVal.([]interface{})
	if !ok {
		return false
	}
	condmap, ok := condVal.(map[string]interface{})
	if !ok {
		return false
	}

	// Decide on the type of check to perform on the attribute values.
	check := func(v interface{}) bool { return evalConditionValue(condVal, v) }
	if !isOperatorObject(condmap) {
		cond := BuildCondition(condmap)
		if cond == nil {
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

	// Check attribute array values.
	for _, a := range attrs {
		if check(a) {
			return true
		}
	}
	return false
}

// Perform "exists" operation.
func existsCheck(condVal interface{}, attrVal interface{}) bool {
	cond, ok := condVal.(bool)
	if !ok {
		return false
	}
	if !cond {
		return attrVal == nil
	}
	return attrVal != nil
}

// Perform "all" operation.
func evalAll(condVal interface{}, attrVal interface{}) bool {
	conds, okc := condVal.([]interface{})
	attrs, oka := attrVal.([]interface{})
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
