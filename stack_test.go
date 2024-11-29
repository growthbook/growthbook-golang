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
