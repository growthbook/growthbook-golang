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
		// string condition: JS `value + "" === condition`
		{"1", 1, true},
		{"1", []any{1}, true},
		{"1", "1", true},
		{"1", true, false},
		{"null", nil, true},
		// number condition: JS `value * 1 === condition`
		{0, "0", true},
		{0, 0, true},
		{0, "", true},
		{0, false, true},
		{0, nil, true},
		{1, "abc", false},
		// boolean condition: JS `value !== null && !!value === condition`
		{false, false, true},
		{false, 0, true},
		{false, nil, false},
		{true, "0", true},
		{true, nil, false},
		// null condition: JS `value === null` (missing attributes read as null)
		{nil, nil, true},
		{nil, "1", false},
		{nil, 0, false},
		{nil, false, false},
	}
	for _, tt := range tests {
		var c Condition = NewValueCond(tt.e)
		require.Equal(t, tt.r, c.Eval(value.New(tt.a), nil), " ValueCond(%v).Eval(%v) == %v", tt.e, tt.a, tt.r)
	}
}
