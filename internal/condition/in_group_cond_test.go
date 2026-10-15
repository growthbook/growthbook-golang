package condition

import (
	"fmt"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

// Both legacy operators use the condition's attribute and ordinary $in semantics,
// regardless of whether the payload entry is a bare array or a typed list.
func TestGroupOperatorsListValues(t *testing.T) {
	for _, entry := range []string{
		`["u1",2]`,
		`{"type":"list","attributeKey":"id","values":["u1",2]}`,
		`{"type":"list","values":["u1",2]}`,
		`{"type":"list","attributeKey":7,"values":["u1",2]}`,
	} {
		t.Run(entry, func(t *testing.T) {
			groups := `{"g":` + entry + `}`
			for _, tc := range []struct {
				actual any
				member bool
			}{
				{"u1", true}, {2, true}, {"2", false}, {"u2", false},
				{[]any{"u2", "u1"}, true}, {[]any{"u2", 2}, true},
				{[]any{"u2", "2"}, false}, {[]any{}, false},
			} {
				attrs := map[string]any{"id": "ignored", "backup_id": tc.actual}
				for _, op := range []string{"$inGroup", "$notInGroup"} {
					cond := fmt.Sprintf(`{"backup_id":{%q:"g"}}`, op)
					want := tc.member
					if op == "$notInGroup" {
						want = !want
					}
					require.Equal(t, want, evalSavedGroupJSON(t, cond, groups, attrs), "%s, attribute=%v", op, tc.actual)
				}
			}
		})
	}
}

func TestGroupOperatorsEmptyList(t *testing.T) {
	for _, entry := range []string{`[]`, `{"type":"list","values":[]}`} {
		groups := `{"g":` + entry + `}`
		attrs := map[string]any{"id": "u1"}
		require.False(t, evalSavedGroupJSON(t, `{"id":{"$inGroup":"g"}}`, groups, attrs))
		require.True(t, evalSavedGroupJSON(t, `{"id":{"$notInGroup":"g"}}`, groups, attrs))
	}
}

func TestInGroupCond(t *testing.T) {
	groups := SavedGroups{
		"test": value.Arr(10, 20, 30),
	}
	test := NewInGroupCond("test")
	nope := NewInGroupCond("nope")
	require.True(t, test.Eval(value.New(10), groups, nil))
	require.False(t, test.Eval(value.New(100), groups, nil))
	require.False(t, nope.Eval(value.New(10), groups, nil))
}

func TestNotInGroupCond(t *testing.T) {
	groups := SavedGroups{
		"test": value.Arr(10, 20, 30),
	}
	test := NewNotInGroupCond("test")
	nope := NewNotInGroupCond("nope")
	require.False(t, test.Eval(value.New(10), groups, nil))
	require.True(t, test.Eval(value.New(100), groups, nil))
	require.True(t, nope.Eval(value.New(10), groups, nil))
}
