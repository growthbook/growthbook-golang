package condition

import (
	"encoding/json"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestElemMatchCondDirect(t *testing.T) {
	cond := NewElemMatchCond(
		AndConds{
			NewCompCond(gtOp, 10),
			NewCompCond(lteOp, 20),
		},
	)
	require.True(t, cond.Eval(value.Arr(1, 2, 4, 15, 30), nil, nil))
	require.False(t, cond.Eval(value.Arr(1, 2, 4, 10, 30), nil, nil))
}

func TestElemMatchCondNested(t *testing.T) {
	cond := NewElemMatchCond(
		NewFieldCond("name", NewCompCond(eqOp, "test")),
	)

	val1 := value.Arr(tag("tag1"), tag("tag2"), tag("tag3"))
	val2 := value.Arr(tag("tag1"), tag("test"), tag("tag3"))
	require.False(t, cond.Eval(val1, nil, nil))
	require.True(t, cond.Eval(val2, nil, nil))
}

func tag(name string) value.ObjValue {
	return value.ObjValue{"name": value.New(name)}
}

// pruneJSON prunes a JSON condition against JSON attributes.
func pruneJSON(t *testing.T, cond, attrs string) any {
	t.Helper()
	var c map[string]any
	require.NoError(t, json.Unmarshal([]byte(cond), &c))
	var a map[string]any
	require.NoError(t, json.Unmarshal([]byte(attrs), &a))
	return Prune(value.New(a).(value.ObjValue), c, nil)
}

// Inside $elemMatch, a nested array element exposes its indexes and length to
// logical conditions, as JS getPath reads an array's own keys.
func TestElemMatchLogicOnNestedArrayElements(t *testing.T) {
	attrs := `{"x": [[1]]}`
	for cond, want := range map[string]bool{
		`{"x": {"$elemMatch": {"$or": [{"0": 1}]}}}`:       true,
		`{"x": {"$elemMatch": {"$nor": [{"0": 1}]}}}`:      false,
		`{"x": {"$elemMatch": {"$and": [{"length": 1}]}}}`: true,
		`{"x": {"$elemMatch": {"$or": [{"1": 1}]}}}`:       false,
	} {
		require.Equal(t, want, parseBase(t, cond).Eval(value.New(map[string]any{"x": []any{[]any{1}}}), nil), cond)
		require.Equal(t, want, pruneJSON(t, cond, attrs), cond)
	}
}

// A malformed branch of a logical condition inside $elemMatch fails only when
// evaluation reaches it, as in JS.
func TestElemMatchLogicMalformedBranchIsLazy(t *testing.T) {
	attrs := `{"x": [{"id": "123"}]}`
	attrsValue := value.New(map[string]any{"x": []any{map[string]any{"id": "123"}}})

	passing := `{"x": {"$elemMatch": {"$or": [{"id": "123"}, {"$not": null}]}}}`
	require.True(t, parseBase(t, passing).Eval(attrsValue, nil))
	require.Equal(t, true, pruneJSON(t, passing, attrs))

	// Reaching the malformed branch fails the whole evaluation and keeps the
	// condition as written
	throwing := `{"x": {"$elemMatch": {"$or": [{"id": "456"}, {"$not": null}]}}}`
	match, threw := parseBase(t, throwing).EvalSafely(attrsValue, nil)
	require.False(t, match)
	require.True(t, threw)
	var want map[string]any
	require.NoError(t, json.Unmarshal([]byte(throwing), &want))
	require.Equal(t, want, pruneJSON(t, throwing, attrs))
}
