package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestCompCond(t *testing.T) {
	tests := []struct {
		op    Operator
		value any
		arg   any
		res   bool
	}{
		{eqOp, 1, 1, true},
		{eqOp, 1, "1", false},
		{neOp, 1, 1, false},
		{neOp, "aa", "bb", true},
		{ltOp, 2, "10", true},
		{ltOp, "2", 2, false},
		{lteOp, "1", 1, true},
		{lteOp, 100, 10, false},
		{gtOp, 10, "2", true},
		{gteOp, value.Null(), 0, true},
	}
	for _, tt := range tests {
		c := NewCompCond(tt.op, tt.arg)
		require.Equal(t, tt.res, c.Eval(value.New(tt.value), nil), "%v %v %v != %v", tt.value, tt.op, tt.arg, tt.res)
	}
}

func TestJsCompare(t *testing.T) {
	lt, eq, gt, er := -1, 0, 1, 2
	vals := []any{value.Null(), true, false, "100", "2", "ABCD", 100, 2, 0}
	tests := map[any][]int{
		value.Null(): {eq, lt, eq, lt, lt, er, lt, lt, eq},
		true:         {gt, eq, gt, lt, lt, er, lt, lt, gt},
		false:        {eq, lt, eq, lt, lt, er, lt, lt, eq},
		"100":        {gt, gt, gt, eq, lt, lt, eq, gt, gt},
		"2":          {gt, gt, gt, gt, eq, lt, lt, eq, gt},
		"ABCD":       {er, er, er, gt, gt, eq, er, er, er},
		100:          {gt, gt, gt, eq, gt, er, eq, gt, gt},
		2:            {gt, gt, gt, lt, eq, er, lt, eq, gt},
		0:            {eq, lt, eq, lt, lt, er, lt, lt, eq},
	}
	for k, v := range tests {
		for i := range vals {
			require.Equal(t, v[i], jsCompare(value.New(k), value.New(vals[i])), "jsCompare(%v, %v) != %v", k, vals[i], v[i])
		}
	}
}
