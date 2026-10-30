package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestVersionCond(t *testing.T) {
	tests := []struct {
		op    Operator
		value any
		arg   any
		res   bool
	}{
		{veqOp, 1, 1, true},
		{veqOp, 1, "1", false},
		{vneOp, 1, "1", true},
		{veqOp, 1.5, "1.5", false},
		{vgtOp, 2, "1.0.0", false},
		{vltOp, nil, "0.0.1", false},
		{veqOp, true, "0", false},
		{veqOp, []any{"1.0.0"}, "0", false},
		{vneOp, nil, "0", true},
		{vneOp, "1.2", "1.2.0", true},
		{vgtOp, "1.2.3", "1.2.3-rc", true},
		{vgteOp, "1.2.3", "1.2.4", false},
		{vltOp, "1.2.3-rc", "1.2.3", true},
		{vlteOp, "1", 1, false},
		{vlteOp, "1", "1", true},
	}
	for _, tt := range tests {
		var c Condition = NewVersionCond(tt.op, value.New(tt.arg))
		require.Equal(t, tt.res, c.Eval(value.New(tt.value), nil, nil), "%v %v %v != %v", tt.arg, tt.op, tt.value, tt.res)
	}
}

func TestPaddedVersion(t *testing.T) {
	tests := map[any]string{
		"v1.1":           "    1-    1",
		"1.2.3":          "    1-    2-    3-~",
		"1.2.3-rc-1.bbb": "    1-    2-    3-rc-    1-bbb",
	}
	for s, r := range tests {
		require.Equal(t, r, paddedVersionString(value.New(s)))
	}
}
