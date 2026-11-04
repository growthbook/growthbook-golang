package condition

import (
	"encoding/json"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func parseBase(t *testing.T, s string) Base {
	t.Helper()
	var b Base
	require.NoError(t, json.Unmarshal([]byte(s), &b))
	return b
}

func TestUnknownOperatorFailsEvenWhenNegated(t *testing.T) {
	attrs := value.New(map[string]any{"name": "x"})
	for _, cond := range []string{
		`{"name": {"$foo": 1}}`,
		`{"$not": {"name": {"$foo": 1}}}`,
		`{"$nor": [{"name": {"$foo": 1}}]}`,
		`{"name": {"$not": {"$foo": 1}}}`,
		`{"$not": {"$or": [{"name": {"$foo": 1}}, {"name": "y"}]}}`,
	} {
		require.False(t, parseBase(t, cond).Eval(attrs, nil), cond)
	}
	// A known operator that fails, or one that passes in an $or, still decides
	require.True(t, parseBase(t, `{"$or": [{"name": {"$foo": 1}}, {"name": "x"}]}`).Eval(attrs, nil))
	require.True(t, parseBase(t, `{"$not": {"name": {"$foo": 1}, "missing": {"$exists": true}}}`).Eval(attrs, nil))
}

func TestConditionOperatorOnValueNeverMatches(t *testing.T) {
	attrs := value.New(map[string]any{"name": "x"})
	require.True(t, parseBase(t, `{"name": {"$not": {"$or": [{"name": "x"}]}}}`).Eval(attrs, nil))
}

func TestNewSavedGroupTypeIsUnknown(t *testing.T) {
	var groups SavedGroups
	require.NoError(t, json.Unmarshal([]byte(`{"g": {"type": "somethingNew"}}`), &groups))
	attrs := value.New(map[string]any{"id": "1"})
	for _, cond := range []string{
		`{"$savedGroup": {"id": "g"}}`,
		`{"$not": {"$savedGroup": {"id": "g"}}}`,
		`{"id": {"$inGroup": "g"}}`,
		`{"id": {"$notInGroup": "g"}}`,
		`{"$not": {"id": {"$inGroup": "g"}}}`,
	} {
		require.False(t, parseBase(t, cond).Eval(attrs, groups), cond)
	}
}

func TestElemMatchConditionOperatorsTestFields(t *testing.T) {
	attrs := value.New(map[string]any{"items": []any{map[string]any{"a": 1}, map[string]any{"a": 2, "b": 3}}})
	require.True(t, parseBase(t, `{"items": {"$elemMatch": {"$or": [{"a": 5}, {"b": 3}]}}}`).Eval(attrs, nil))
	require.True(t, parseBase(t, `{"items": {"$elemMatch": {"$and": [{"a": 2}, {"b": 3}]}}}`).Eval(attrs, nil))
	require.False(t, parseBase(t, `{"items": {"$elemMatch": {"$and": [{"a": 1}, {"b": 3}]}}}`).Eval(attrs, nil))
}

func TestElemMatchSkipsNullElements(t *testing.T) {
	attrs := value.New(map[string]any{"items": []any{nil}})
	require.False(t, parseBase(t, `{"items": {"$elemMatch": {"$exists": false}}}`).Eval(attrs, nil))
	attrs = value.New(map[string]any{"items": []any{nil, 0}})
	require.True(t, parseBase(t, `{"items": {"$elemMatch": {"$eq": 0}}}`).Eval(attrs, nil))
}

func TestMalformedConditionNeverMatches(t *testing.T) {
	for _, cond := range []string{`{"$not": null}`, `{"$and": [null]}`, `{"x": {"$elemMatch": 5}}`, `5`} {
		b := parseBase(t, cond)
		require.True(t, b.Malformed(), cond)
		require.False(t, b.Eval(value.New(map[string]any{}), nil), cond)
	}
	b := parseBase(t, `null`)
	require.False(t, b.Malformed())
	require.True(t, b.Eval(value.New(map[string]any{}), nil))
}
