package value

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestValueCreation(t *testing.T) {
	t.Run("Null", func(t *testing.T) {
		require.Equal(t, Null(), Null())
		require.True(t, IsNull(Null()))
	})

	t.Run("Bool", func(t *testing.T) {
		require.Equal(t, True(), True())
		require.Equal(t, False(), False())
		require.NotEqual(t, True(), False())
		require.True(t, IsBool(Bool(true)))
	})

	t.Run("Num", func(t *testing.T) {
		require.Equal(t, Num(10), Num(10.0))
		require.NotEqual(t, Num(10.0), Num(10.1))
		require.True(t, IsNum(Num(10)))
	})

	t.Run("Str", func(t *testing.T) {
		require.Equal(t, Str("test"), Str("test"))
		require.NotEqual(t, Str("test"), Str("notest"))
		require.True(t, IsStr(Str("test")))
	})

	t.Run("Arr", func(t *testing.T) {
		require.True(t, IsArr(Arr(10, Num(20), Str("test"))))
	})

	t.Run("Obj", func(t *testing.T) {
		obj := Obj(map[string]any{
			"n": Num(10),
			"s": Str("test"),
			"b": True(),
			"a": Arr(1, "test"),
			"o": ObjValue{"id": Num(10), "name": Str("Object10")},
		})
		require.True(t, IsObj(obj))
	})
}

func TestNew(t *testing.T) {
	type myint int
	type myuint uint
	type myfloat float64
	type mybool bool
	type mystring string

	tests := []struct {
		name     string
		expected Value
		input    any
	}{
		{"Num from int", Num(1), 1},
		{"Num from float", Num(10), 10.0},
		{"Num from custom int", Num(10), myint(10)},
		{"Num from custom float", Num(10.1), myfloat(10.1)},
		{"Num from uint", Num(10), uint(10)},
		{"Num from custom uint", Num(10), myuint(10)},

		{"Bool from bool", True(), true},
		{"Bool from custom bool", False(), mybool(false)},

		{"Str from string", Str("test"), "test"},
		{"Str from custom String", Str("test"), mystring("test")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, New(test.input))
		})
	}
}
