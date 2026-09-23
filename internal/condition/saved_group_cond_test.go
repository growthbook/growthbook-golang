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

// Invalid references fail closed for both group types, including explicit null overrides.
func TestSavedGroupInvalidReferences(t *testing.T) {
	for _, reference := range []string{
		`"g"`, `null`, `true`, `7`, `[]`, `["g"]`, `{}`,
		`{"id":null}`, `{"id":7}`, `{"id":[]}`, `{"id":{}}`,
		`{"id":"g","attributeKey":null}`, `{"id":"g","attributeKey":7}`,
		`{"id":"g","attributeKey":false}`, `{"id":"g","attributeKey":[]}`,
		`{"id":"g","attributeKey":{}}`,
	} {
		t.Run(reference, func(t *testing.T) {
			for _, entry := range []string{
				`{"type":"list","attributeKey":"id","values":["u1"]}`,
				`{"type":"condition","condition":{"id":"u1"}}`,
			} {
				require.False(t, evalSavedGroupJSON(t, `{"$savedGroup":`+reference+`}`, `{"g":`+entry+`}`, map[string]any{"id": "u1"}))
			}
		})
	}
}

// Overrides use the same path and membership rules as an entry's own attribute.
func TestSavedGroupAttributeOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		attrs     map[string]any
		want      bool
	}{
		{"nested", "user.id", map[string]any{"user": map[string]any{"id": "u1"}}, true},
		{"array index", "users.0.id", map[string]any{"users": []any{map[string]any{"id": "u1"}}}, true},
		{"array intersection", "tags", map[string]any{"tags": []any{"u0", "u1"}}, true},
		{"array length", "tags.length", map[string]any{"tags": []any{"a", "b"}}, true},
		{"empty key", "", map[string]any{"": "u1"}, true},
		{"missing override attribute", "missing", map[string]any{"id": "u1"}, false},
		{"scalar intermediate", "user.id", map[string]any{"user": "u1", "id": "u1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cond := fmt.Sprintf(`{"$savedGroup":{"id":"g","attributeKey":%q}}`, tc.key)
			for _, entry := range []string{
				`{"type":"list","attributeKey":"id","values":["u1",2]}`,
				`{"type":"list","values":["u1",2]}`,
				`{"type":"list","attributeKey":null,"values":["u1",2]}`,
				`{"type":"list","attributeKey":7,"values":["u1",2]}`,
			} {
				require.Equal(t, tc.want, evalSavedGroupJSON(t, cond, `{"g":`+entry+`}`, tc.attrs))
			}
		})
	}
}

func TestSavedGroupReferenceIsolation(t *testing.T) {
	groups := `{"list":{"type":"list","attributeKey":"id","values":["u1"]},"condition":{"type":"condition","condition":{"$savedGroup":{"id":"list"}}}}`
	attrs := map[string]any{"id": "u1", "backup_id": "u2"}
	// A condition group's valid override is ignored, not inherited by nested references.
	require.True(t, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"condition","attributeKey":"backup_id"}}`, groups, attrs))
	// An override must not modify the shared list definition used by a sibling.
	require.True(t, evalSavedGroupJSON(t, `{"$and":[{"$not":{"$savedGroup":{"id":"list","attributeKey":"backup_id"}}},{"$savedGroup":{"id":"list"}}]}`, groups, attrs))
	// Future fields are ignored for both types and retained when serializing a condition.
	for _, id := range []string{"list", "condition"} {
		raw := fmt.Sprintf(`{"$savedGroup":{"id":%q,"future":{"nested":[1,true]}}}`, id)
		require.True(t, evalSavedGroupJSON(t, raw, groups, attrs))
		var cond Base
		require.NoError(t, json.Unmarshal([]byte(raw), &cond))
		encoded, err := json.Marshal(cond)
		require.NoError(t, err)
		require.JSONEq(t, raw, string(encoded))
	}
}

// Error markers retain ordinary boolean composition rather than being ignored.
func TestSavedGroupErrorMarkers(t *testing.T) {
	for _, marker := range []string{"__sgInvalid__", "__sgUnknown__", "__sgCycle__", "__sgMaxDepth__"} {
		t.Run(marker, func(t *testing.T) {
			cond := fmt.Sprintf(`{%q:"g"}`, marker)
			require.False(t, evalSavedGroupJSON(t, cond, `{}`, map[string]any{}))
			require.True(t, evalSavedGroupJSON(t, `{"$not":`+cond+`}`, `{}`, map[string]any{}))
			require.False(t, evalSavedGroupJSON(t, `{"$and":[{},`+cond+`]}`, `{}`, map[string]any{}))
			require.True(t, evalSavedGroupJSON(t, `{"$or":[`+cond+`,{}]}`, `{}`, map[string]any{}))
		})
	}
}

func TestSavedGroupMalformedEntries(t *testing.T) {
	entries := []string{
		`null`, `false`, `7`, `"group"`, `{}`,
		`{"type":"future"}`, `{"type":1}`,
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
			for _, cond := range []string{`{"$savedGroup":{"id":"bad"}}`, `{"$savedGroup":{"id":"bad","attributeKey":"id"}}`, `{"id":{"$inGroup":"bad"}}`, `{"id":{"$notInGroup":"bad"}}`} {
				require.False(t, evalSavedGroupJSON(t, cond, groups, attrs), cond)
			}
			require.True(t, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"good"}}`, groups, attrs))
			require.True(t, evalSavedGroupJSON(t, `{"id":{"$notInGroup":"missing"}}`, groups, attrs))
		})
	}
}

func TestSavedGroupRequiresAttributeKey(t *testing.T) {
	// Legacy operators only need values, but $savedGroup must know which attribute to read.
	for _, entry := range []string{
		`{"type":"list","values":["u1"]}`,
		`{"type":"list","attributeKey":7,"values":["u1"]}`,
	} {
		require.False(t, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"g"}}`, `{"g":`+entry+`}`, map[string]any{"id": "u1"}))
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
			require.Equal(t, tc.want, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"g"}}`, groups, tc.attrs))
		})
	}
}

func TestSavedGroupCyclesAndSiblingBranches(t *testing.T) {
	for _, tc := range []struct {
		name, definition string
		want             bool
	}{
		{"direct", `{"$savedGroup":{"id":"g"}}`, false},
		{"and", `{"$and":[{"$savedGroup":{"id":"g"}}]}`, false},
		{"or", `{"$or":[{"$savedGroup":{"id":"g"}}]}`, false},
		{"nor", `{"$nor":[{"$savedGroup":{"id":"g"}}]}`, true},
		{"not", `{"$not":{"$savedGroup":{"id":"g"}}}`, true},
		{"double not", `{"$not":{"$not":{"$savedGroup":{"id":"g"}}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := `{"g":{"type":"condition","condition":` + tc.definition + `}}`
			// A cyclic reference is false; surrounding boolean operators retain
			// their ordinary semantics, including negation.
			require.Equal(t, tc.want, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"g"}}`, groups, map[string]any{}))
		})
	}
	groups := `{"a":{"type":"condition","condition":{"$savedGroup":{"id":"b"}}},"b":{"type":"condition","condition":{"id":"u1"}}}`
	for _, cond := range []string{
		`{"$and":[{"$savedGroup":{"id":"a"}},{"$savedGroup":{"id":"a"}}]}`,
		`{"$or":[{"$and":[{"$savedGroup":{"id":"a"}},{"other":true}]},{"$savedGroup":{"id":"a"}}]}`,
	} {
		require.True(t, evalSavedGroupJSON(t, cond, groups, map[string]any{"id": "u1"}))
	}
}

func TestSavedGroupVisitedThroughNestedValues(t *testing.T) {
	// The second visit would match "terminal" if an array or value operator
	// discarded the visited set on the way to the nested reference.
	for _, op := range []string{"$elemMatch", "$all", "$alli"} {
		t.Run(op, func(t *testing.T) {
			nested := `{"$elemMatch":{"marker":true,"$savedGroup":{"id":"g"}}}`
			var items any = []any{map[string]any{"marker": true, "terminal": true}}
			if op != "$elemMatch" {
				nested = `{"` + op + `":[` + nested + `]}`
				items = []any{items}
			}
			groups := `{"g":{"type":"condition","condition":{"$or":[{"terminal":true},{"items":` + nested + `}]}}}`
			require.False(t, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"g"}}`, groups, map[string]any{"items": items}))
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
	raw := `{"legacy":["u1",2],"list":{"type":"list","attributeKey":"id","values":["u1"]},"condition":{"type":"condition","condition":{"$savedGroup":{"id":"list"}}},"future":{"type":"future","extra":true},"invalid":null}`
	var groups SavedGroups
	require.NoError(t, json.Unmarshal([]byte(raw), &groups))
	encoded, err := json.Marshal(groups)
	require.NoError(t, err)
	require.JSONEq(t, raw, string(encoded))
	require.True(t, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"condition"}}`, string(encoded), map[string]any{"id": "u1"}))
	require.NoError(t, json.Unmarshal([]byte(`null`), &groups))
	require.Nil(t, groups)
}

func TestSavedGroupStructuredValueMismatch(t *testing.T) {
	for _, entry := range []string{
		`{"type":"condition","condition":{"profile":{"a":true}}}`,
		`{"type":"list","attributeKey":"profile","values":[{"a":true}]}`,
	} {
		require.False(t, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"g"}}`, `{"g":`+entry+`}`, map[string]any{"profile": map[string]any{"b": true}}))
	}
}

func TestSavedGroupArrayLengthPath(t *testing.T) {
	require.True(t, evalSavedGroupJSON(t, `{"$savedGroup":{"id":"g"}}`, `{"g":{"type":"list","attributeKey":"tags.length","values":[2]}}`, map[string]any{"tags": []any{"a", "b"}}))
}
