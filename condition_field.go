package growthbook

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

type fieldOp string

const (
	eqOp  fieldOp = "$eq"
	neqOp fieldOp = "$neq"
)

var (
	errCondUnknownFieldOperator = errors.New("Condition unknown field operator")
)

type condField struct {
	path string
	ops  map[fieldOp]any
}

type condFieldOp func(c *Client, value any) bool

func (cond *condField) eval(c *Client, attributes Attributes) bool {
	value := getPath(cond.path, attributes)
	for op, expected := range cond.ops {
		if !evalOp(c, op, expected, value) {
			return false
		}
	}
	return true
}

func evalOp(c *Client, op fieldOp, expected any, value any) bool {
	switch op {
	case eqOp:
		return jsEqual(value, expected)
	case neqOp:
		return !jsEqual(value, expected)
	default:
		c.logger.Warn("Unknown condition op", "op", op)
		return false
	}
}

func getPath(path string, attributes Attributes) any {
	parts := strings.Split(path, ".")
	var current any = attributes
	for _, name := range parts {
		m, ok := current.(Attributes)
		if !ok {
			return nil
		}
		current = m[name]
	}
	return current
}

func (cond *condField) UnmarshalJSON(data []byte) error {
	var arg any
	err := json.Unmarshal(data, &arg)
	if err != nil {
		return err
	}
	switch arg.(type) {
	case map[string]any:
		argMap := arg.(map[string]any)
		return parseCondFieldMap(cond, argMap)
	default:
		cond.ops = map[fieldOp]any{eqOp: arg}
		return nil
	}
}

func parseCondFieldMap(cond *condField, arg map[string]any) error {
	if !isOperatorObject(arg) {
		cond.ops[eqOp] = arg
		return nil
	}
	for k, v := range arg {
		op := fieldOp(k)
		switch op {
		case eqOp, neqOp:
			cond.ops[op] = v
		default:
			return errCondUnknownFieldOperator
		}
	}
	return nil
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

// Equality on values derived from JSON data, following JavaScript
// number comparison rules. This compares arrays/slices (derived from
// JSON arrays), string-keyed maps (derived from JSON objects) and
// atomic values, treating all numbers as floating point, so that "2"
// as an integer compares equal to "2.0", for example. This gets
// around the problem where the Go JSON package decodes all numbers as
// float64, but users may want to use integer values for attributes
// within their Go code, and we would like them to compare equal,
// since that's what happens in the JS SDK.

func jsEqual(a any, b any) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}
	switch reflect.TypeOf(a).Kind() {
	case reflect.Array, reflect.Slice:
		aa, aok := a.([]any)
		ba, bok := b.([]any)
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
		am, aok := a.(map[string]any)
		bm, bok := b.(map[string]any)
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

func normalizeNumber(a any) any {
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
