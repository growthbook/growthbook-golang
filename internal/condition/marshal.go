package condition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/value"
)

type Base struct {
	cond      Condition
	raw       json.RawMessage
	malformed bool
}

// Eval starts a condition evaluation with no saved-group references visited.
// Only a condition that passes matches: one that depends on an operator or
// saved group type this SDK doesn't support fails, and so does one whose
// evaluation reaches a malformed part.
func (base Base) Eval(actual value.Value, groups SavedGroups) bool {
	match, _ := base.EvalSafely(actual, groups)
	return match
}

// EvalSafely is Eval that also reports whether the evaluation reached a
// malformed part, where the JS SDK throws.
func (base Base) EvalSafely(actual value.Value, groups SavedGroups) (match bool, threw bool) {
	if base.cond == nil {
		return true, false
	}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(malformedPart); !ok {
				panic(r)
			}
			match, threw = false, true
		}
	}()
	return eval3(base.cond, actual, groups, nil) == pass, false
}

// Malformed reports whether the condition has a part that could not be
// parsed. Evaluation fails when it reaches that part.
func (base Base) Malformed() bool {
	return base.malformed
}

// UnmarshalJSON parses a condition. A malformed part, such as `{"$not": null}`,
// fails the condition when evaluation reaches it instead of failing the whole
// payload, as in the JS SDK, where feature and experiment evaluation fail a
// condition that throws.
func (base *Base) UnmarshalJSON(data []byte) error {
	if string(bytes.TrimSpace(data)) == "null" {
		*base = Base{}
		return nil
	}
	m := map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		*base = malformedBase(data)
		return nil
	}
	cond, malformed, err := buildLazyBaseCond(value.New(m))
	if err != nil {
		*base = malformedBase(data)
		return nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	*base = Base{cond: cond, raw: raw, malformed: malformed}
	return nil
}

// malformedPart is the panic value of throwCond, recovered by Base.EvalSafely.
type malformedPart struct{}

// throwCond stands in for a part of a condition that could not be parsed.
// Reaching it fails the whole evaluation, like a thrown error in the JS SDK,
// so it can't be negated into a match.
type throwCond struct{}

func (throwCond) Eval(value.Value, SavedGroups, visitedGroups) bool {
	panic(malformedPart{})
}

func (throwCond) eval3(value.Value, SavedGroups, visitedGroups) result {
	panic(malformedPart{})
}

// buildLazyBaseCond builds a condition whose malformed entries, including
// those in $and, $or, $nor and $not, fail only when evaluation reaches them.
// It reports whether there were any.
func buildLazyBaseCond(json value.Value) (Condition, bool, error) {
	obj, ok := json.(value.ObjValue)
	if !ok {
		return nil, false, fmt.Errorf("base expected to be an object")
	}
	malformed := false
	conds := []Condition{}
	for _, f := range orderedConditionKeys(obj) {
		cond, m, err := buildLazyLogicCond(f, obj[f])
		if err != nil {
			cond, m = throwCond{}, true
		}
		malformed = malformed || m
		conds = append(conds, cond)
	}
	if len(conds) == 1 {
		return conds[0], malformed, nil
	}
	return AndConds(conds), malformed, nil
}

func buildLazyLogicCond(op string, arg value.Value) (Condition, bool, error) {
	switch Operator(op) {
	case andOp, orOp, norOp:
		arr, ok := arg.(value.ArrValue)
		if !ok {
			return nil, false, fmt.Errorf("error parsing `%v` condition: array expected", op)
		}
		malformed := false
		conds := make([]Condition, 0, len(arr))
		for _, v := range arr {
			cond, m, err := buildLazyBaseCond(v)
			if err != nil {
				cond, m = throwCond{}, true
			}
			malformed = malformed || m
			conds = append(conds, cond)
		}
		return newLogicCond(op, conds), malformed, nil
	case notOp:
		cond, malformed, err := buildLazyBaseCond(arg)
		if err != nil {
			return nil, false, fmt.Errorf("error parsing `%v` condition: %w", op, err)
		}
		return NotCond{cond}, malformed, nil
	}
	cond, err := buildLogicCond(op, arg)
	return cond, false, err
}

// MarshalJSON emits the parsed condition in canonical form, so conditions
// survive a marshal/unmarshal round trip instead of collapsing to {}. A zero
// Base marshals as {} — the always-true condition, matching its Eval — which
// is also what it marshaled as before conditions carried their raw form.
func (base Base) MarshalJSON() ([]byte, error) {
	if base.raw == nil {
		return []byte("{}"), nil
	}
	return base.raw, nil
}

func buildBaseCond(json value.Value) (Condition, error) {
	obj, ok := json.(value.ObjValue)
	if !ok {
		return nil, fmt.Errorf("Base expected to be an object")
	}
	conds := []Condition{}
	for _, f := range orderedConditionKeys(obj) {
		fv := obj[f]
		cond, err := buildLogicCond(f, fv)
		if err != nil {
			return nil, fmt.Errorf("Error building %v : %v", f, err)
		}
		conds = append(conds, cond)
	}
	if len(conds) == 1 {
		return conds[0], nil
	}
	return AndConds(conds), nil
}

func orderedConditionKeys(obj value.ObjValue) []string {
	keys := make([]string, 0, len(obj))
	seen := make(map[string]bool, len(obj))
	for _, op := range []Operator{andOp, orOp, norOp, notOp} {
		key := string(op)
		if _, ok := obj[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var fields []string
	for key := range obj {
		if !seen[key] {
			fields = append(fields, key)
		}
	}
	sort.Strings(fields)
	return append(keys, fields...)
}

func buildLogicCond(op string, arg value.Value) (Condition, error) {
	switch Operator(op) {
	case savedGroupOp:
		reference, ok := arg.(value.ObjValue)
		if !ok {
			return False{}, nil
		}
		id, ok := reference["id"].(value.StrValue)
		if !ok {
			return False{}, nil
		}
		cond := savedGroupCond{id: string(id)}
		if key, present := reference["attributeKey"]; present {
			attributeKey, ok := key.(value.StrValue)
			if !ok {
				return False{}, nil
			}
			cond.overridePath = strings.Split(string(attributeKey), ".")
		}
		return cond, nil
	case "$savedGroups":
		// The plural operator is authoring-only and never valid on the wire.
		return False{}, nil
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

func buildValueCondCaseInsensitive(json value.Value) (Condition, error) {
	obj, ok := json.(value.ObjValue)
	if !(ok && isOperatorObject(obj)) {
		return NewValueCondCaseInsensitive(json), nil
	}

	return buildObjCond(obj)
}

func buildObjCond(obj value.ObjValue) (Condition, error) {
	var conds []Condition
	for _, op := range orderedOperatorKeys(obj) {
		arg := obj[op]
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

func orderedOperatorKeys(obj value.ObjValue) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
	case iniOp:
		arr, ok := arg.(value.ArrValue)
		if !ok {
			return False{}, nil
		}
		return NewIniCond(arr), nil
	case niniOp:
		arr, ok := arg.(value.ArrValue)
		if !ok {
			return False{}, nil
		}
		return NewNotIniCond(arr), nil
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
	case regexiOp:
		return buildRegexiCond(arg)
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
	case alliOp:
		return buildAlliConds(arg)
	case notOp:
		cond, err := buildValueCond(arg)
		if err != nil {
			return nil, fmt.Errorf("Error parsing $not operator: %w", err)
		}
		return NotCond{cond}, nil
	case andOp, orOp, norOp, savedGroupOp:
		// Keys this SDK knows, but on a condition rather than on a value,
		// where they never match
		return False{}, nil
	default:
		return unknownOpCond{}, nil
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

func buildRegexiCond(arg value.Value) (Condition, error) {
	s, ok := arg.(value.StrValue)
	if !ok {
		return nil, fmt.Errorf("RegexiOp argument %v isn't a string", arg)
	}

	// Add case-insensitive flag to the pattern
	pattern := "(?i)" + string(s)
	r, err := regexp.Compile(pattern)
	if err != nil {
		return False{}, nil
	}
	return NewRegexiCond(r), nil
}

func buildElemMatchCond(arg value.Value) (Condition, error) {
	obj, ok := arg.(value.ObjValue)
	if !ok {
		return nil, fmt.Errorf("ElemMatch arg is not an object: %v", arg)
	}
	// Operators test each element itself. A condition, including one that only
	// uses $or, $and, $nor, or $savedGroup, tests each element's fields.
	if isOperatorObject(obj) && !hasConditionOperator(obj) {
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

func hasConditionOperator(obj value.ObjValue) bool {
	for _, op := range []Operator{orOp, norOp, andOp, savedGroupOp} {
		if _, ok := obj[string(op)]; ok {
			return true
		}
	}
	return false
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

func buildAlliConds(arg value.Value) (Condition, error) {
	arr, ok := arg.(value.ArrValue)
	if !ok {
		return nil, fmt.Errorf("$alli arg %v is not an array", arg)
	}
	res := AlliConds{}
	for _, v := range arr {
		c, err := buildValueCondCaseInsensitive(v)
		if err != nil {
			return nil, fmt.Errorf("$alli arg is invalid: %w", err)
		}
		res = append(res, c)
	}
	return res, nil
}

// malformedBase stands in for a condition that isn't an object. It fails when
// evaluated and marshals back to raw.
func malformedBase(raw json.RawMessage) Base {
	return Base{cond: throwCond{}, raw: append(json.RawMessage(nil), raw...), malformed: true}
}
