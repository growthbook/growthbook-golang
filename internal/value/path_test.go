package value

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPath(t *testing.T) {
	obj := ObjValue{"items": ArrValue{ObjValue{"id": Str("u1")}}, "01": Str("key"), "": True()}
	for _, tc := range []struct {
		name string
		path []string
		want Value
	}{
		{"root", nil, obj},
		{"array leaf", []string{"items"}, obj["items"]},
		{"object leaf", []string{"items", "0"}, ObjValue{"id": Str("u1")}},
		{"scalar leaf", []string{"items", "0", "id"}, Str("u1")},
		{"past scalar", []string{"items", "0", "id", "name"}, Null()},
		{"array length", []string{"items", "length"}, Num(1)},
		{"past length", []string{"items", "length", "id"}, Null()},
		{"missing key", []string{"missing"}, Null()},
		{"out of bounds", []string{"items", "1"}, Null()},
		{"negative index", []string{"items", "-1"}, Null()},
		{"leading zero", []string{"items", "00"}, Null()},
		{"explicit plus", []string{"items", "+0"}, Null()},
		{"nonnumeric index", []string{"items", "id"}, Null()},
		{"numeric object key", []string{"01"}, Str("key")},
		{"empty key", []string{""}, True()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, Path(obj, tc.path...))
			require.Equal(t, tc.want, obj.Path(tc.path...))
		})
	}
	require.Equal(t, Str("u1"), Path(obj["items"], "0", "id"))
}
