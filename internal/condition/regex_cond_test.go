package condition

import (
	"regexp"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestRegexCond(t *testing.T) {
	rx := regexp.MustCompile(".*test.*")
	var c Condition = NewRegexCond(rx)
	require.True(t, c.Eval(value.New("some test string"), nil))
	require.False(t, c.Eval(value.New("some string"), nil))
}

func TestRegexiCond(t *testing.T) {
	t.Run("case-insensitive match", func(t *testing.T) {
		rx := regexp.MustCompile("(?i).*test.*")
		var c Condition = NewRegexiCond(rx)
		require.True(t, c.Eval(value.New("some test string"), nil))
		require.True(t, c.Eval(value.New("some TEST string"), nil))
		require.True(t, c.Eval(value.New("some TeSt string"), nil))
		require.False(t, c.Eval(value.New("some string"), nil))
	})

	t.Run("non-string values", func(t *testing.T) {
		rx := regexp.MustCompile("(?i)test")
		var c Condition = NewRegexiCond(rx)
		require.False(t, c.Eval(value.New(123), nil))
		require.False(t, c.Eval(value.New(true), nil))
		require.False(t, c.Eval(value.New(nil), nil))
	})
}
