package condition

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func evalSavedGroupJSON(t *testing.T, conditionJSON, groupsJSON string, attributes any) bool {
	t.Helper()
	var cond Base
	var groups SavedGroups
	require.NoError(t, json.Unmarshal([]byte(conditionJSON), &cond))
	require.NoError(t, json.Unmarshal([]byte(groupsJSON), &groups))
	return cond.Eval(value.New(attributes), groups)
}

func TestSavedGroupMalformedEntries(t *testing.T) {
	entries := []string{
		`null`, `false`, `7`, `"group"`, `{}`,
		`{"type":"future"}`, `{"type":1}`,
		`{"type":"list","values":["u1"]}`,
		`{"type":"list","attributeKey":7,"values":["u1"]}`,
		`{"type":"list","attributeKey":"id"}`,
		`{"type":"list","attributeKey":"id","values":null}`,
		`{"type":"list","attributeKey":"id","values":{}}`,
		`{"type":"condition"}`, `{"type":"condition","condition":null}`,
		`{"type":"condition","condition":[]}`,
		`{"type":"condition","condition":true}`,
		`{"type":"condition","condition":{"$and":7}}`,
		`{"type":"condition","condition":{"id":{"$regex":7}}}`,
	}
	for _, entry := range entries {
		t.Run(entry, func(t *testing.T) {
			groups := `{"bad":` + entry + `,"good":{"type":"condition","condition":{}}}`
			attrs := map[string]any{"id": "u1"}
			for _, cond := range []string{`{"$savedGroup":"bad"}`, `{"id":{"$inGroup":"bad"}}`, `{"id":{"$notInGroup":"bad"}}`} {
				require.False(t, evalSavedGroupJSON(t, cond, groups, attrs), cond)
			}
			require.True(t, evalSavedGroupJSON(t, `{"$savedGroup":"good"}`, groups, attrs))
			require.True(t, evalSavedGroupJSON(t, `{"id":{"$notInGroup":"missing"}}`, groups, attrs))
		})
	}
}

func TestSavedGroupAttributePaths(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		attrs      any
		want       bool
	}{
		{"nested", "user.id", map[string]any{"user": map[string]any{"id": "u1"}}, true},
		{"array intersection", "user.tags", map[string]any{"user": map[string]any{"tags": []any{"u0", "u1"}}}, true},
		{"array index", "users.0.id", map[string]any{"users": []any{map[string]any{"id": "u1"}}}, true},
		{"missing path", "user.id", map[string]any{}, false},
		{"scalar intermediate", "user.id", map[string]any{"user": "u1"}, false},
		{"empty key", "", map[string]any{"": "u1"}, true},
		{"strict membership", "id", map[string]any{"id": 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := fmt.Sprintf(`{"g":{"type":"list","attributeKey":%q,"values":["u1","1"]}}`, tc.path)
			require.Equal(t, tc.want, evalSavedGroupJSON(t, `{"$savedGroup":"g"}`, groups, tc.attrs))
		})
	}
}

func TestSavedGroupCyclesAndSiblingBranches(t *testing.T) {
	for _, tc := range []struct {
		name, definition string
		want             bool
	}{
		{"direct", `{"$savedGroup":"g"}`, false},
		{"and", `{"$and":[{"$savedGroup":"g"}]}`, false},
		{"or", `{"$or":[{"$savedGroup":"g"}]}`, false},
		{"nor", `{"$nor":[{"$savedGroup":"g"}]}`, true},
		{"not", `{"$not":{"$savedGroup":"g"}}`, true},
		{"double not", `{"$not":{"$not":{"$savedGroup":"g"}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := `{"g":{"type":"condition","condition":` + tc.definition + `}}`
			// A cyclic reference is false; surrounding boolean operators retain
			// their ordinary semantics, including negation.
			require.Equal(t, tc.want, evalSavedGroupJSON(t, `{"$savedGroup":"g"}`, groups, map[string]any{}))
		})
	}
	groups := `{"a":{"type":"condition","condition":{"$savedGroup":"b"}},"b":{"type":"condition","condition":{"id":"u1"}}}`
	for _, cond := range []string{
		`{"$and":[{"$savedGroup":"a"},{"$savedGroup":"a"}]}`,
		`{"$or":[{"$and":[{"$savedGroup":"a"},{"other":true}]},{"$savedGroup":"a"}]}`,
	} {
		require.True(t, evalSavedGroupJSON(t, cond, groups, map[string]any{"id": "u1"}))
	}
}

func TestSavedGroupVisitedThroughNestedValues(t *testing.T) {
	// The second visit would match "terminal" if an array or value operator
	// discarded the visited set on the way to the nested reference.
	for _, op := range []string{"$elemMatch", "$all", "$alli"} {
		t.Run(op, func(t *testing.T) {
			nested := `{"$elemMatch":{"marker":true,"$savedGroup":"g"}}`
			var items any = []any{map[string]any{"marker": true, "terminal": true}}
			if op != "$elemMatch" {
				nested = `{"` + op + `":[` + nested + `]}`
				items = []any{items}
			}
			groups := `{"g":{"type":"condition","condition":{"$or":[{"terminal":true},{"items":` + nested + `}]}}}`
			require.False(t, evalSavedGroupJSON(t, `{"$savedGroup":"g"}`, groups, map[string]any{"items": items}))
		})
	}
	// $size cannot contain a top-level reference in valid wire JSON, but its
	// nested condition must still forward the evaluation's guard.
	groups := SavedGroups{"g": savedGroup{cond: True{}}}
	seen := visitedGroups{"g": {}}
	require.False(t, NewSizeCond(savedGroupCond{id: "g"}).Eval(value.Arr(1), groups, seen))
	require.False(t, (NotCond{NotCond{savedGroupCond{id: "g"}}}).Eval(value.Null(), groups, seen))
}

func TestSavedGroupLongAcyclicChain(t *testing.T) {
	groups := make(SavedGroups)
	for i := 0; i < 150; i++ {
		groups[fmt.Sprint(i)] = savedGroup{cond: savedGroupCond{id: fmt.Sprint(i + 1)}}
	}
	groups["150"] = savedGroup{cond: True{}}
	require.True(t, (savedGroupCond{id: "0"}).Eval(value.Null(), groups, nil))
}

func TestSavedGroupsJSONRoundTrip(t *testing.T) {
	raw := `{"legacy":["u1",2],"list":{"type":"list","attributeKey":"id","values":["u1"]},"condition":{"type":"condition","condition":{"$savedGroup":"list"}},"future":{"type":"future","extra":true},"invalid":null}`
	var groups SavedGroups
	require.NoError(t, json.Unmarshal([]byte(raw), &groups))
	encoded, err := json.Marshal(groups)
	require.NoError(t, err)
	require.JSONEq(t, raw, string(encoded))
	require.True(t, evalSavedGroupJSON(t, `{"$savedGroup":"condition"}`, string(encoded), map[string]any{"id": "u1"}))
	require.NoError(t, json.Unmarshal([]byte(`null`), &groups))
	require.Nil(t, groups)
}

func TestSavedGroupStructuredValueMismatch(t *testing.T) {
	for _, entry := range []string{
		`{"type":"condition","condition":{"profile":{"a":true}}}`,
		`{"type":"list","attributeKey":"profile","values":[{"a":true}]}`,
	} {
		require.False(t, evalSavedGroupJSON(t, `{"$savedGroup":"g"}`, `{"g":`+entry+`}`, map[string]any{"profile": map[string]any{"b": true}}))
	}
}

func TestSavedGroupArrayLengthPath(t *testing.T) {
	require.True(t, evalSavedGroupJSON(t, `{"$savedGroup":"g"}`, `{"g":{"type":"list","attributeKey":"tags.length","values":[2]}}`, map[string]any{"tags": []any{"a", "b"}}))
}
