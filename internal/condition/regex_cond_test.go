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
