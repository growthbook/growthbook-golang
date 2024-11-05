package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestTypeCond(t *testing.T) {
	tests := []struct {
		t string
		v value.Value
		r bool
	}{
		{"boolean", value.New(true), true},
		{"boolean", value.New("true"), false},
		{"number", value.New(10), true},
		{"number", value.New("10"), false},
		{"string", value.New("test"), true},
		{"null", value.Null(), true},
		{"null", value.New(""), false},
		{"array", value.Arr("1", 2), true},
		{"array", value.New("[1,2]"), false},
		{"object", value.ObjValue{}, true},
		{"object", value.ArrValue{}, false},
	}
	for _, tt := range tests {
		var c Condition = NewTypeCond(tt.t)
		require.Equal(t, tt.r, c.Eval(tt.v, nil), "%v not of type %v", tt.v, tt.t)
	}

}
