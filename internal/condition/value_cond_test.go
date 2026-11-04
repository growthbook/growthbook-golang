package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestValueCond(t *testing.T) {
	tests := []struct {
		e any
		a any
		r bool
	}{
		// values match only with the same type and value; no coercion
		{"1", 1, false},
		{"1", []any{1}, false},
		{"1", "1", true},
		{"1", true, false},
		{"null", nil, false},
		{0, "0", false},
		{0, 0, true},
		{0, "", false},
		{0, false, false},
		{0, nil, false},
		{1, "abc", false},
		{false, false, true},
		{false, 0, false},
		{false, "", false},
		{false, nil, false},
		{true, "0", false},
		{true, 1, false},
		{true, nil, false},
		// null condition: JS `value === null` (missing attributes read as null)
		{nil, nil, true},
		{nil, "1", false},
		{nil, 0, false},
		{nil, false, false},
		{[]any{1}, []any{1}, true},
		{[]any{1}, []any{"1"}, false},
	}
	for _, tt := range tests {
		var c Condition = NewValueCond(tt.e)
		require.Equal(t, tt.r, c.Eval(value.New(tt.a), nil, nil), " ValueCond(%v).Eval(%v) == %v", tt.e, tt.a, tt.r)
	}
}
