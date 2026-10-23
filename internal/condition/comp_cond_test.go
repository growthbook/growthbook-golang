package condition

import (
	"encoding/json"
	"math"
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
		{ltOp, 2, 10, true},
		{ltOp, "10", "9", true},
		{ltOp, 2, "10", false},
		{ltOp, "2", 2, false},
		{lteOp, "1", 1, false},
		{lteOp, 100, 10, false},
		{gtOp, 10, "2", false},
		{gtOp, "2026-10-22", "2026-09-30", true},
		{gtOp, true, false, false},
		{gteOp, true, true, false},
		{gteOp, value.Null(), 0, false},
		{gteOp, value.Null(), value.Null(), false},
	}
	for _, tt := range tests {
		c := NewCompCond(tt.op, tt.arg)
		require.Equal(t, tt.res, c.Eval(value.New(tt.value), nil, nil), "%v %v %v != %v", tt.value, tt.op, tt.arg, tt.res)
	}
}

// cases.json can't encode NaN, so these mirror JS, where NaN is unordered
// and not equal to itself
func TestCompCondNaN(t *testing.T) {
	nan := math.NaN()
	tests := []struct {
		op    Operator
		value any
		arg   any
		res   bool
	}{
		{ltOp, nan, 0, false},
		{lteOp, nan, 0, false},
		{gtOp, nan, 0, false},
		{gteOp, nan, 0, false},
		{lteOp, 0, nan, false},
		{gteOp, nan, nan, false},
		{eqOp, nan, nan, false},
		{neOp, nan, nan, true},
		{neOp, nan, 0, true},
	}
	for _, tt := range tests {
		c := NewCompCond(tt.op, tt.arg)
		require.Equal(t, tt.res, c.Eval(value.New(tt.value), nil, nil), "%v %v %v != %v", tt.value, tt.op, tt.arg, tt.res)
	}
}

func TestNaNAttributeFailsNumericLimit(t *testing.T) {
	var c Base
	require.NoError(t, json.Unmarshal([]byte(`{"score":{"$lte":0}}`), &c))
	require.False(t, c.Eval(value.ObjValue{"score": value.Num(math.NaN())}, nil))
	require.True(t, c.Eval(value.ObjValue{"score": value.Num(-1)}, nil))
}

func TestCompare(t *testing.T) {
	lt, eq, gt := -1, 0, 1
	tests := []struct {
		a, b any
		res  int
	}{
		{"100", "2", lt},
		{"2", "2", eq},
		{"ABCD", "100", gt},
		{100, 2, gt},
		{2, 2, eq},
		{0, 2, lt},
	}
	for _, tt := range tests {
		require.Equal(t, tt.res, compare(value.New(tt.a), value.New(tt.b)), "compare(%v, %v) != %v", tt.a, tt.b, tt.res)
	}
}
