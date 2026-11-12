package growthbook

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStack(t *testing.T) {
	stack := &stack[string]{}
	require.False(t, stack.has("test"))
	stack.push("1")
	require.True(t, stack.has("1"))
	stack.push("2")
	require.True(t, stack.has("1"))
	require.True(t, stack.has("2"))
	res, ok := stack.pop()
	require.Equal(t, "2", res)
	require.True(t, ok)
	require.False(t, stack.has("2"))
	require.True(t, stack.has("1"))
}

func TestStackGrowsPastInline(t *testing.T) {
	s := &stack[int]{}
	n := stackInlineSize*2 + 1
	for i := 0; i < n; i++ {
		s.push(i)
	}
	for i := 0; i < n; i++ {
		require.True(t, s.has(i))
	}
	for i := n - 1; i >= 0; i-- {
		v, ok := s.pop()
		require.True(t, ok)
		require.Equal(t, i, v)
	}
	_, ok := s.pop()
	require.False(t, ok)
}
