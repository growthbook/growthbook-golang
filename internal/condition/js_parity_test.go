package condition

import (
	"encoding/json"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

// TestJSParity covers spec 0.12.0 behaviour that the shared cases don't.
func TestJSParity(t *testing.T) {
	tests := []struct {
		name  string
		cond  string
		attrs string
		want  bool
	}{
		{"$eq array operand is never equal", `{"x":{"$eq":[1]}}`, `{"x":[1]}`, false},
		{"$ne array operand is always unequal", `{"x":{"$ne":[1]}}`, `{"x":[1]}`, true},
		{"$eq object operand is never equal", `{"x":{"$eq":{"a":1}}}`, `{"x":{"a":1}}`, false},
		{"$ne object operand is always unequal", `{"x":{"$ne":{"a":1}}}`, `{"x":{"a":1}}`, true},
		{"$in array item is never equal", `{"x":{"$in":[[1]]}}`, `{"x":[1]}`, false},
		{"$nin array item is never equal", `{"x":{"$nin":[[1]]}}`, `{"x":[1]}`, true},
		{"$ini object item is never equal", `{"x":{"$ini":[{"a":1}]}}`, `{"x":{"a":1}}`, false},
		{"$nini object item is never equal", `{"x":{"$nini":[{"a":1}]}}`, `{"x":{"a":1}}`, true},
		{"$regex number pattern", `{"x":{"$regex":1}}`, `{"x":"1"}`, false},
		{"$regexi null pattern", `{"x":{"$regexi":null}}`, `{"x":"null"}`, false},
		{"$regex array pattern", `{"x":{"$regex":["a"]}}`, `{"x":"a"}`, false},
		{"1 equals 1.0 in $in", `{"x":{"$in":[1.0]}}`, `{"x":1}`, true},
		{"1 equals 1.0 in $eq on array", `{"x":{"$all":[1.0]}}`, `{"x":[1]}`, true},
		{"1 equals 1.0 in $elemMatch", `{"x":{"$elemMatch":{"$eq":1.0}}}`, `{"x":[1]}`, true},
		{"missing $eq null", `{"x":{"$eq":null}}`, `{}`, true},
		{"missing $ne null", `{"x":{"$ne":null}}`, `{}`, false},
		{"missing $type null", `{"x":{"$type":"null"}}`, `{}`, true},
		{"missing $type undefined", `{"x":{"$type":"undefined"}}`, `{}`, false},
		{"null $type undefined", `{"x":{"$type":"undefined"}}`, `{"x":null}`, false},
		{"missing $type unknown name", `{"x":{"$type":"foo"}}`, `{}`, false},
		{"strings order by UTF-16 code units", `{"x":{"$lt":"\uffff"}}`, `{"x":"\ud83d\ude00"}`, true},
		{"strings order by UTF-16 code units (gt)", `{"x":{"$gt":"\ud83d\ude00"}}`, `{"x":"\uffff"}`, true},
		{"missing $exists true", `{"x":{"$exists":true}}`, `{}`, false},
		{"missing $nin", `{"x":{"$nin":["a"]}}`, `{}`, true},
		{"missing $nini", `{"x":{"$nini":["a"]}}`, `{}`, true},
		{"empty $nor", `{"$nor":[]}`, `{"x":1}`, false},
		{"array index path", `{"tags.0":"a"}`, `{"tags":["a","b"]}`, true},
		{"array index path miss", `{"tags.1":"a"}`, `{"tags":["a","b"]}`, false},
		{"nested array index path", `{"x.1.y":2}`, `{"x":[{"y":1},{"y":2}]}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Base
			require.NoError(t, json.Unmarshal([]byte(tt.cond), &c))
			var attrs map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.attrs), &attrs))
			require.Equal(t, tt.want, c.Eval(value.Obj(attrs), nil))
		})
	}
}

// Saved group values compare numbers by value, so 1 matches 1.0.
func TestJSParitySavedGroupNumbers(t *testing.T) {
	var c Base
	require.NoError(t, json.Unmarshal([]byte(`{"x":{"$inGroup":"g"}}`), &c))
	var groups SavedGroups
	require.NoError(t, json.Unmarshal([]byte(`{"g":[1.0,2]}`), &groups))
	require.True(t, c.Eval(value.Obj(map[string]any{"x": 1}), groups))
	require.True(t, c.Eval(value.Obj(map[string]any{"x": 2.0}), groups))
}
