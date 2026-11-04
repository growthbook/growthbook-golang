package growthbook

import (
	"encoding/json"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestParentConditionMalformedNeverMatches(t *testing.T) {
	var p ParentCondition
	require.NoError(t, json.Unmarshal([]byte(`{"id":"parent","condition":{"$not":null},"gate":true}`), &p))
	require.Equal(t, "parent", p.Id)
	require.True(t, p.Gate)
	require.False(t, p.Condition.Eval(value.New(map[string]any{"value": true}), nil))

	data, err := json.Marshal(p.Condition)
	require.NoError(t, err)
	require.JSONEq(t, `{"$not":null}`, string(data))
}

func TestParentConditionMissingConditionMatches(t *testing.T) {
	var p ParentCondition
	require.NoError(t, json.Unmarshal([]byte(`{"id":"parent","condition":null}`), &p))
	require.True(t, p.Condition.Eval(value.New(map[string]any{"value": true}), nil))
}
