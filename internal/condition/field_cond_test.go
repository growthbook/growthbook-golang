package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestFieldCond(t *testing.T) {
	eq20 := NewCompCond(eqOp, 20)
	c := NewFieldCond("user.age", eq20)
	obj1 := value.ObjValue{"user": value.ObjValue{"age": value.Num(20)}}
	obj2 := value.ObjValue{"user": value.ObjValue{"name": value.Str("Bob")}}
	require.True(t, c.Eval(obj1, nil))
	require.False(t, c.Eval(obj2, nil))
}
