package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestInGroupCond(t *testing.T) {
	groups := SavedGroups{
		"test": value.Arr(10, 20, 30),
	}
	test := NewInGroupCond("test")
	nope := NewInGroupCond("nope")
	require.True(t, test.Eval(value.New(10), groups))
	require.False(t, test.Eval(value.New(100), groups))
	require.False(t, nope.Eval(value.New(10), groups))
}

func TestNotInGroupCond(t *testing.T) {
	groups := SavedGroups{
		"test": value.Arr(10, 20, 30),
	}
	test := NewNotInGroupCond("test")
	nope := NewNotInGroupCond("nope")
	require.False(t, test.Eval(value.New(10), groups))
	require.True(t, test.Eval(value.New(100), groups))
	require.True(t, nope.Eval(value.New(10), groups))
}
