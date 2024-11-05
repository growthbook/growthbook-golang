package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestInCond(t *testing.T) {
	t.Run("empty arr returns false", func(t *testing.T) {
		c := NewInCond(value.Arr())
		require.False(t, c.Eval(value.New(100), nil))
	})
	t.Run("search in array casts to value type", func(t *testing.T) {
		c := NewInCond(value.Arr(1, 200, 100))
		require.True(t, c.Eval(value.New("100"), nil))
		require.True(t, c.Eval(value.New(true), nil))
		require.True(t, c.Eval(value.New(200), nil))
		require.False(t, c.Eval(value.New(400), nil))
	})
}

func TestNotInCond(t *testing.T) {
	t.Run("empty arr returns true", func(t *testing.T) {
		c := NewNotInCond(value.Arr())
		require.True(t, c.Eval(value.New(100), nil))
	})
	t.Run("search in array casts to value type", func(t *testing.T) {
		c := NewNotInCond(value.Arr(1, 200, 100))
		require.False(t, c.Eval(value.New("100"), nil))
		require.False(t, c.Eval(value.New(true), nil))
		require.False(t, c.Eval(value.New(200), nil))
		require.True(t, c.Eval(value.New(400), nil))
	})
}
