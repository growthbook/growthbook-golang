package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestInCond(t *testing.T) {
	t.Run("empty arr returns false", func(t *testing.T) {
		c := NewInCond(value.Arr())
		require.False(t, c.Eval(value.New(100), nil, nil))
	})
	t.Run("search in array uses strict equality", func(t *testing.T) {
		c := NewInCond(value.Arr(1, 200, 100))
		require.False(t, c.Eval(value.New("100"), nil, nil))
		require.False(t, c.Eval(value.New(true), nil, nil))
		require.True(t, c.Eval(value.New(200), nil, nil))
		require.False(t, c.Eval(value.New(400), nil, nil))
	})
	t.Run("all-string array uses set lookup", func(t *testing.T) {
		c := NewInCond(value.Arr("a", "b", "c"))
		require.NotNil(t, c.strSet)
		require.True(t, c.Eval(value.New("a"), nil, nil))
		require.True(t, c.Eval(value.New("c"), nil, nil))
		require.False(t, c.Eval(value.New("d"), nil, nil))
		require.False(t, c.Eval(value.New("1"), nil, nil))
		require.False(t, c.Eval(value.New(1), nil, nil))
		require.True(t, c.Eval(value.Arr("x", "b"), nil, nil))
		require.False(t, c.Eval(value.Arr("x", "y"), nil, nil))
	})
	t.Run("mixed-type array falls back to linear scan", func(t *testing.T) {
		c := NewInCond(value.Arr("a", 1, "c"))
		require.Nil(t, c.strSet)
		require.True(t, c.Eval(value.New("a"), nil, nil))
		require.True(t, c.Eval(value.New(1), nil, nil))
		require.False(t, c.Eval(value.New("b"), nil, nil))
	})
}

func TestNotInCond(t *testing.T) {
	t.Run("empty arr returns true", func(t *testing.T) {
		c := NewNotInCond(value.Arr())
		require.True(t, c.Eval(value.New(100), nil, nil))
	})
	t.Run("search in array uses strict equality", func(t *testing.T) {
		c := NewNotInCond(value.Arr(1, 200, 100))
		require.True(t, c.Eval(value.New("100"), nil, nil))
		require.True(t, c.Eval(value.New(true), nil, nil))
		require.False(t, c.Eval(value.New(200), nil, nil))
		require.True(t, c.Eval(value.New(400), nil, nil))
	})
}

func TestIniCond(t *testing.T) {
	t.Run("case-insensitive string matching", func(t *testing.T) {
		c := NewIniCond(value.Arr("apple", "BANANA", "Cherry"))
		require.True(t, c.Eval(value.New("apple"), nil, nil))
		require.True(t, c.Eval(value.New("APPLE"), nil, nil))
		require.True(t, c.Eval(value.New("banana"), nil, nil))
		require.True(t, c.Eval(value.New("cherry"), nil, nil))
		require.True(t, c.Eval(value.New("CHERRY"), nil, nil))
		require.False(t, c.Eval(value.New("grape"), nil, nil))
	})

	t.Run("array attribute values", func(t *testing.T) {
		c := NewIniCond(value.Arr("apple", "BANANA"))
		require.True(t, c.Eval(value.Arr("APPLE", "orange"), nil, nil))
		require.True(t, c.Eval(value.Arr("grape", "banana"), nil, nil))
		require.False(t, c.Eval(value.Arr("grape", "orange"), nil, nil))
	})

	t.Run("non-string types use exact equality", func(t *testing.T) {
		c := NewIniCond(value.Arr(1, 2, 3))
		require.True(t, c.Eval(value.New(1), nil, nil))
		require.True(t, c.Eval(value.New(2), nil, nil))
		require.False(t, c.Eval(value.New(4), nil, nil))
	})

	t.Run("empty array returns false", func(t *testing.T) {
		c := NewIniCond(value.Arr())
		require.False(t, c.Eval(value.New("test"), nil, nil))
	})

	t.Run("all-string array uses set lookup", func(t *testing.T) {
		c := NewIniCond(value.Arr("Alpha", "BETA"))
		require.NotNil(t, c.strSet)
		require.True(t, c.Eval(value.New("alpha"), nil, nil))
		require.True(t, c.Eval(value.New("Beta"), nil, nil))
		require.False(t, c.Eval(value.New("gamma"), nil, nil))
		require.False(t, c.Eval(value.New(1), nil, nil))
	})

	t.Run("non-ascii strings fall back to EqualFold scan", func(t *testing.T) {
		c := NewIniCond(value.Arr("CAFÉ"))
		require.Nil(t, c.strSet)
		require.True(t, c.Eval(value.New("café"), nil, nil))
		require.False(t, c.Eval(value.New("cafe"), nil, nil))
	})

	t.Run("non-ascii actual values fall back to EqualFold scan", func(t *testing.T) {
		c := NewIniCond(value.Arr("k"))
		require.NotNil(t, c.strSet)
		require.True(t, c.Eval(value.New("K"), nil, nil))
		require.True(t, c.Eval(value.New("K"), nil, nil))
	})

	t.Run("mixed-type array falls back to linear scan", func(t *testing.T) {
		c := NewIniCond(value.Arr("Alpha", 1))
		require.Nil(t, c.strSet)
		require.True(t, c.Eval(value.New("ALPHA"), nil, nil))
		require.True(t, c.Eval(value.New(1), nil, nil))
	})
}

func TestNotIniCond(t *testing.T) {
	t.Run("case-insensitive string matching", func(t *testing.T) {
		c := NewNotIniCond(value.Arr("apple", "BANANA"))
		require.False(t, c.Eval(value.New("apple"), nil, nil))
		require.False(t, c.Eval(value.New("APPLE"), nil, nil))
		require.False(t, c.Eval(value.New("banana"), nil, nil))
		require.True(t, c.Eval(value.New("grape"), nil, nil))
	})

	t.Run("empty array returns true", func(t *testing.T) {
		c := NewNotIniCond(value.Arr())
		require.True(t, c.Eval(value.New("test"), nil, nil))
	})

	t.Run("inverts ascii set lookup", func(t *testing.T) {
		c := NewNotIniCond(value.Arr("Alpha", "BETA"))
		require.False(t, c.Eval(value.New("alpha"), nil, nil))
		require.True(t, c.Eval(value.New("gamma"), nil, nil))
	})
}
