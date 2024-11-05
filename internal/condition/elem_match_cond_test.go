package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestElemMatchCondDirect(t *testing.T) {
	cond := NewElemMatchCond(
		AndConds{
			NewCompCond(gtOp, 10),
			NewCompCond(lteOp, 20),
		},
	)
	require.True(t, cond.Eval(value.Arr(1, 2, 4, 15, 30), nil))
	require.False(t, cond.Eval(value.Arr(1, 2, 4, 10, 30), nil))
}

func TestElemMatchCondNested(t *testing.T) {
	cond := NewElemMatchCond(
		NewFieldCond("name", NewCompCond(eqOp, "test")),
	)

	val1 := value.Arr(tag("tag1"), tag("tag2"), tag("tag3"))
	val2 := value.Arr(tag("tag1"), tag("test"), tag("tag3"))
	require.False(t, cond.Eval(val1, nil))
	require.True(t, cond.Eval(val2, nil))
}

func tag(name string) value.ObjValue {
	return value.ObjValue{"name": value.New(name)}
}
