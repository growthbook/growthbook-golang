package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

type Const value.BoolValue

func (c Const) Eval(obj value.ObjValue) bool {
	return value.BoolValue(c) == value.True()
}

func TestBaseEval(t *testing.T) {
	obj := value.ObjValue{}

	c := Base{}
	require.True(t, c.Eval(value.ObjValue{}))

	ct := Const(value.True())
	cf := Const(value.False())
	cs := Base{ct, cf}
	require.False(t, cs.Eval(obj))

	cs = Base{
		And{
			Base{ct},
			Base{Not{cf}, Or{Base{ct}, Base{cf}}},
		},
	}
	require.True(t, cs.Eval(obj))
}
