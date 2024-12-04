package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestSizeCond(t *testing.T) {
	var c Condition = NewSizeCond(NewValueCond(3))
	require.True(t, c.Eval(value.Arr(10, 20, 30), nil))
	require.False(t, c.Eval(value.Arr(), nil))
	require.False(t, c.Eval(value.Arr(1), nil))
}
