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

func TestIniCond(t *testing.T) {
	t.Run("case-insensitive string matching", func(t *testing.T) {
		c := NewIniCond(value.Arr("apple", "BANANA", "Cherry"))
		require.True(t, c.Eval(value.New("apple"), nil))
		require.True(t, c.Eval(value.New("APPLE"), nil))
		require.True(t, c.Eval(value.New("banana"), nil))
		require.True(t, c.Eval(value.New("cherry"), nil))
		require.True(t, c.Eval(value.New("CHERRY"), nil))
		require.False(t, c.Eval(value.New("grape"), nil))
	})

	t.Run("array attribute values", func(t *testing.T) {
		c := NewIniCond(value.Arr("apple", "BANANA"))
		require.True(t, c.Eval(value.Arr("APPLE", "orange"), nil))
		require.True(t, c.Eval(value.Arr("grape", "banana"), nil))
		require.False(t, c.Eval(value.Arr("grape", "orange"), nil))
	})

	t.Run("non-string types use exact equality", func(t *testing.T) {
		c := NewIniCond(value.Arr(1, 2, 3))
		require.True(t, c.Eval(value.New(1), nil))
		require.True(t, c.Eval(value.New(2), nil))
		require.False(t, c.Eval(value.New(4), nil))
	})

	t.Run("empty array returns false", func(t *testing.T) {
		c := NewIniCond(value.Arr())
		require.False(t, c.Eval(value.New("test"), nil))
	})
}

func TestNotIniCond(t *testing.T) {
	t.Run("case-insensitive string matching", func(t *testing.T) {
		c := NewNotIniCond(value.Arr("apple", "BANANA"))
		require.False(t, c.Eval(value.New("apple"), nil))
		require.False(t, c.Eval(value.New("APPLE"), nil))
		require.False(t, c.Eval(value.New("banana"), nil))
		require.True(t, c.Eval(value.New("grape"), nil))
	})

	t.Run("empty array returns true", func(t *testing.T) {
		c := NewNotIniCond(value.Arr())
		require.True(t, c.Eval(value.New("test"), nil))
	})
}
