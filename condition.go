package growthbook

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
)

// Condition represents conditions used to target features/experiments
// to specific users. Wraps condition interface to allow for JSON
// marshalling.
type Condition struct {
	c cond
}

// Eval evaluates a condition on a set of attributes.
func (c Condition) Eval(attrs Attributes) bool {
	return c.c.eval(attrs)
}

// MarshalJSON for conditions.
func (c Condition) MarshalJSON() ([]byte, error) {
	return c.c.MarshalJSON()
}

// UnmarshalJSON for conditions: mostly hands off to concrete
// unmarshallers for subconditions.
func (c *Condition) UnmarshalJSON(data []byte) error {
	topLevel := make(map[string]json.RawMessage)
	err := json.Unmarshal(data, &topLevel)
	if err != nil {
		return err
	}

	subCond := func(key string, maker func() cond) (bool, error) {
		sub, ok := topLevel[key]
		if !ok {
			// Not present, no error.
			return false, nil
		}
		// Make subcondition value.
		tmp := maker()
		err = json.Unmarshal(sub, tmp)
		if err != nil {
			// Present, error.
			return true, err
		}
		c.c = tmp
		// Present, no error.
		return true, nil
	}

	used, err := subCond("$or", func() cond { return &orCond{} })
	if used {
		return err
	}
	used, err = subCond("$nor", func() cond { return &norCond{} })
	if used {
		return err
	}
	used, err = subCond("$and", func() cond { return &andCond{} })
	if used {
		return err
	}
	used, err = subCond("$not", func() cond { return &notCond{} })
	if used {
		return err
	}

	tmp := baseCond{}
	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	c.c = &tmp
	return nil
}

// Internal condition interface.
type cond interface {
	json.Marshaler
	eval(attrs Attributes) bool
}

// Concrete condition representing ORing together a list of
// conditions.
type orCond struct {
	conds []cond
}

// Evaluate ORed list of conditions.
func (c orCond) eval(attrs Attributes) bool {
	if len(c.conds) == 0 {
		return true
	}
	for i := range c.conds {
		if c.conds[i].eval(attrs) {
			return true
		}
	}
	return false
}

// MarshalJSON serializes OR conditions to JSON.
func (c orCond) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string][]cond{"$or": c.conds})
}

// UnmarshalJSON deserializes OR conditions from JSON.
func (c *orCond) UnmarshalJSON(data []byte) error {
	tmp := []Condition{}
	err := json.Unmarshal(data, &tmp)
	if err == nil {
		c.conds = make([]cond, len(tmp))
		for i := range tmp {
			c.conds[i] = tmp[i].c
		}
	}
	return err
}

// Concrete condition representing NORing together a list of
// conditions.
type norCond struct {
	conds []cond
}

// Evaluate NORed list of conditions.
func (c norCond) eval(attrs Attributes) bool {
	or := orCond{c.conds}
	return !or.eval(attrs)
}

// MarshalJSON serializes NOR conditions to JSON.
func (c norCond) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string][]cond{"$nor": c.conds})
}

// UnmarshalJSON deserializes NOR conditions from JSON.
func (c *norCond) UnmarshalJSON(data []byte) error {
	tmp := []Condition{}
	err := json.Unmarshal(data, &tmp)
	if err == nil {
		c.conds = make([]cond, len(tmp))
		for i := range tmp {
			c.conds[i] = tmp[i].c
		}
	}
	return err
}

// Concrete condition representing ANDing together a list of
// conditions.
type andCond struct {
	conds []cond
}

// Evaluate ANDed list of conditions.
func (c andCond) eval(attrs Attributes) bool {
	for i := range c.conds {
		if !c.conds[i].eval(attrs) {
			return false
		}
	}
	return true
}

// MarshalJSON serializes AND conditions to JSON.
func (c andCond) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string][]cond{"$and": c.conds})
}

// UnmarshalJSON deserializes AND conditions from JSON.
func (c *andCond) UnmarshalJSON(data []byte) error {
	tmp := []Condition{}
	err := json.Unmarshal(data, &tmp)
	if err == nil {
		c.conds = make([]cond, len(tmp))
		for i := range tmp {
			c.conds[i] = tmp[i].c
		}
	}
	return err
}

// Concrete condition representing the complement of another
// condition.
type notCond struct {
	cond cond
}

// Evaluate complemented condition.
func (c notCond) eval(attrs Attributes) bool {
	return !c.cond.eval(attrs)
}

// MarshalJSON serializes NOT conditions to JSON.
func (c notCond) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]cond{"$not": c.cond})
}

// UnmarshalJSON deserializes NOT conditions from JSON.
func (c *notCond) UnmarshalJSON(data []byte) error {
	tmp := Condition{}
	err := json.Unmarshal(data, &tmp)
	if err == nil {
		c.cond = tmp.c
	}
	return err
}

// Concrete condition representing the base condition case of a set of
// keys and values or subsidiary conditions.
type baseCond struct {
	// This is represented in this dynamically typed form to make lax
	// error handling easier.
	values map[string]interface{}
}

// Evaluate base Condition case by iterating over keys and performing
// evaluation for each one (either a simple comparison, or an operator
// evaluation).
func (c baseCond) eval(attrs Attributes) bool {
	for k, v := range c.values {
		if !evalConditionValue(v, getPath(attrs, k)) {
			return false
		}
	}
	return true
}

// MarshalJSON serializes base conditions to JSON.
func (c baseCond) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.values)
}

// UnmarshalJSON deserializes base conditions from JSON.
func (c *baseCond) UnmarshalJSON(data []byte) error {
	tmp := map[string]interface{}{}
	err := json.Unmarshal(data, &tmp)
	if err == nil {
		c.values = tmp
	}
	return err
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

	return jsEqual(condVal, attrVal)
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
	case "$veq", "$vne", "$vgt", "$vgte", "$vlt", "$vlte":
		attrstring, attrok := attrVal.(string)
		condstring, reok := condVal.(string)
		if !reok || !attrok {
			return false
		}
		return versionCompare(key, attrstring, condstring)

	case "$eq":
		return jsEqual(attrVal, condVal)

	case "$ne":
		return !jsEqual(attrVal, condVal)

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
		vals, ok := condVal.([]interface{})
		if !ok {
			return false
		}
		return elementIn(attrVal, vals)

	case "$nin":
		vals, ok := condVal.([]interface{})
		if !ok {
			return false
		}
		return !elementIn(attrVal, vals)

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

// Perform version string comparisons.

func versionCompare(comp string, v1 string, v2 string) bool {
	v1 = paddedVersionString(v1)
	v2 = paddedVersionString(v2)
	switch comp {
	case "$veq":
		return v1 == v2
	case "$vne":
		return v1 != v2
	case "$vgt":
		return v1 > v2
	case "$vgte":
		return v1 >= v2
	case "$vlt":
		return v1 < v2
	case "$vlte":
		return v1 <= v2
	}
	return false
}

// Perform numeric or string ordering comparisons on polymorphic JSON
// values.
func compare(comp string, x interface{}, y interface{}) bool {
	switch x.(type) {
	case float64:
		xn := x.(float64)
		yn, ok := y.(float64)
		if !ok {
			logger.Warn("Types don't match in condition comparison operation")
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
			logger.Warn("Types don't match in condition comparison operation")
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

// Check for membership of a JSON value in a JSON array or
// intersection of two arrays.

func elementIn(v interface{}, array []interface{}) bool {
	otherArray, ok := v.([]interface{})
	if ok {
		// Both arguments are arrays, so look for intersection.
		return commonElement(array, otherArray)
	}

	// One single value, one array, so do membership test.
	for _, val := range array {
		if jsEqual(v, val) {
			return true
		}
	}
	return false
}

// Check for common element in two arrays.

func commonElement(a1 []interface{}, a2 []interface{}) bool {
	for _, el1 := range a1 {
		for _, el2 := range a2 {
			if reflect.DeepEqual(el1, el2) {
				return true
			}
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
	conddata, err := json.Marshal(condmap)
	if err != nil {
		return false
	}

	// Decide on the type of check to perform on the attribute values.
	check := func(v interface{}) bool { return evalConditionValue(condVal, v) }
	if !isOperatorObject(condmap) {
		cond := Condition{}
		err := json.Unmarshal(conddata, &cond)
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

// Equality on values derived from JSON data, following JavaScript
// number comparison rules. This compares arrays/slices (derived from
// JSON arrays), string-keyed maps (derived from JSON objects) and
// atomic values, treating all numbers as floating point, so that "2"
// as an integer compares equal to "2.0", for example. This gets
// around the problem where the Go JSON package decodes all numbers as
// float64, but users may want to use integer values for attributes
// within their Go code, and we would like them to compare equal,
// since that's what happens in the JS SDK.

func jsEqual(a interface{}, b interface{}) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}
	switch reflect.TypeOf(a).Kind() {
	case reflect.Array, reflect.Slice:
		aa, aok := a.([]interface{})
		ba, bok := b.([]interface{})
		if !aok || !bok {
			return false
		}
		if len(aa) != len(ba) {
			return false
		}
		for i, av := range aa {
			if !jsEqual(av, ba[i]) {
				return false
			}
		}
		return true

	case reflect.Map:
		am, aok := a.(map[string]interface{})
		bm, bok := b.(map[string]interface{})
		if !aok || !bok {
			return false
		}
		if len(am) != len(bm) {
			return false
		}
		for k, av := range am {
			bv, ok := bm[k]
			if !ok {
				return false
			}
			if !jsEqual(av, bv) {
				return false
			}
		}
		return true

	default:
		return reflect.DeepEqual(normalizeNumber(a), normalizeNumber(b))
	}
}

func normalizeNumber(a interface{}) interface{} {
	v := reflect.ValueOf(a)
	if v.CanFloat() {
		return v.Float()
	}
	if v.CanInt() {
		return float64(v.Int())
	}
	if v.CanUint() {
		return float64(v.Uint())
	}
	return a
}
