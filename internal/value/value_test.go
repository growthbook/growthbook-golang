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

func TestCast(t *testing.T) {
	tests := []struct {
		name     string
		expected Value
		input    Value
		vtype    ValueType
	}{
		// Analog !!arg JS expression
		{"Null to Bool", False(), Null(), BoolType},
		{"Bool to Bool", True(), True(), BoolType},
		{"Num to True", True(), Num(1), BoolType},
		{"Num to False", False(), Num(0), BoolType},
		{"Str to True", True(), Str("test"), BoolType},
		{"Str to False", False(), Str(""), BoolType},
		{"Arr To Bool", True(), ArrValue{}, BoolType},
		{"Obj To Bool", True(), ObjValue{}, BoolType},

		// Analog of arg * 1 JS expression
		{"Null to Num", Num(0), Null(), NumType},
		{"True to Num", Num(1), True(), NumType},
		{"False to Num", Num(0), False(), NumType},
		{"Num to Num", Num(10), Num(10), NumType},
		{"Empty Str to Num", Num(0), Str(""), NumType},
		{"Number Str to Num", Num(10), Str("10"), NumType},
		{"Number Str to Num 2", Num(10.1), Str("  10.1  "), NumType},
		{"Non number Str to Num", Null(), Str("bbb"), NumType},
		{"Empty Arr To Num", Num(0), Arr(), NumType},
		{"Arr with one elem to Num", Num(10), Arr("10"), NumType},
		{"Arr with non num elem to Num", Null(), Arr("bla"), NumType},
		{"Arr with many elems to Num", Null(), Arr(1, 2), NumType},
		{"Obj to Num", Null(), ObjValue{}, NumType},

		// Analog of arg + "" JS expression
		{"Null to Str", Str("null"), Null(), StrType},
		{"True to Str", Str("true"), True(), StrType},
		{"False to Str", Str("false"), False(), StrType},
		{"Number to Str", Str("10.1"), Num(10.1), StrType},
		{"Str to Str", Str("test"), Str("test"), StrType},
		{"Empty Arr to Str", Str(""), Arr(), StrType},
		{"Arr to Str", Str("1,2,3,test,,10,20"), Arr(1, 2, 3, "test", Arr(), Arr(10, 20)), StrType},
		{"Obj to Str", Null(), ObjValue{}, StrType},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, test.input.Cast(test.vtype))
		})
	}

}
