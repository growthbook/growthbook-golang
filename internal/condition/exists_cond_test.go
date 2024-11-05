package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestExistsCond(t *testing.T) {
	tests := []struct {
		expected bool
		value    any
		res      bool
	}{
		{true, 1, true},
		{true, value.Null(), false},
		{false, value.Null(), true},
		{false, "AA", false},
	}
	for _, tt := range tests {
		cond := NewExistsCond(tt.expected)
		require.Equal(t, tt.res, cond.Eval(value.New(tt.value), nil))
	}
}
