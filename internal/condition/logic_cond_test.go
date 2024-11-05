package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

type Const value.BoolValue

func (c Const) Eval(_ value.Value, _ SavedGroups) bool {
	return value.BoolValue(c) == value.True()
}

var (
	ct = Const(value.True())
	cf = Const(value.False())
)

func TestOr(t *testing.T) {
	empty := OrConds{}
	require.True(t, empty.Eval(value.Null(), nil))

	c1 := OrConds{ct, cf}
	require.True(t, c1.Eval(value.Null(), nil))

	c2 := OrConds{cf, cf}
	require.False(t, c2.Eval(value.Null(), nil))
}

func TestAnd(t *testing.T) {
	empty := AndConds{}
	require.True(t, empty.Eval(value.Null(), nil))

	c1 := AndConds{ct, cf}
	require.False(t, c1.Eval(value.Null(), nil))

	c2 := AndConds{ct, ct}
	require.True(t, c2.Eval(value.Null(), nil))
}

func TestNot(t *testing.T) {
	c := NotCond{ct}
	require.False(t, c.Eval(value.Null(), nil))
}

func TestNor(t *testing.T) {
	empty := NorConds{}
	require.False(t, empty.Eval(value.Null(), nil))

	c1 := NorConds{ct, cf}
	require.False(t, c1.Eval(value.Null(), nil))

	c2 := NorConds{cf, cf}
	require.True(t, c2.Eval(value.Null(), nil))
}
