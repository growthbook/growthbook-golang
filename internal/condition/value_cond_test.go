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
		{"1", 1, false},
		{"1", []any{1}, false},
		{"1", "1", true},
		{"1", true, false},
		{0, "0", false},
		{0, 0, true},
		{0, "", false},
		{0, false, false},
		{false, false, true},
		{false, nil, false},
		{nil, nil, true},
		{nil, "1", false},
	}
	for _, tt := range tests {
		var c Condition = NewValueCond(tt.e)
		require.Equal(t, tt.r, c.Eval(value.New(tt.a), nil), " ValueCond(%v).Eval(%v) == %v", tt.e, tt.a, tt.r)
	}
}
