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
		{veqOp, 1, "1", true},
		{vneOp, "1.2", "1.2.0", true},
		{vgtOp, "1.2.3", "1.2.3-rc", true},
		{vgteOp, "1.2.3", "1.2.4", false},
		{vgteOp, "1.02.3", "1.2.4", false},
		{veqOp, "1.02.3", "1.2.3", true},
		{vltOp, "1.2.3-rc", "1.2.3", true},
		{vlteOp, "1", 1, true},
	}
	for _, tt := range tests {
		var c Condition = NewVersionCond(tt.op, value.New(tt.arg))
		require.Equal(t, tt.res, c.Eval(value.New(tt.value), nil), "%v %v %v != %v", tt.arg, tt.op, tt.value, tt.res)
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
