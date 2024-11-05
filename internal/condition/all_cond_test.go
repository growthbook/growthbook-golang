package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestAllConds(t *testing.T) {
	eq1 := NewCompCond(eqOp, 1)
	eq2 := NewCompCond(eqOp, 2)
	gt10 := NewCompCond(gtOp, 10)

	cond := AllConds{eq1, eq2, gt10}
	require.True(t, cond.Eval(value.Arr(2, 20, 1, 5), nil))
	require.False(t, cond.Eval(value.Arr(1, 5, 1, 50), nil))
}
