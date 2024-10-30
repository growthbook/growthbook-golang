package value

import (
	"strings"
)

type ArrValue []Value

func Arr(args ...any) ArrValue {
	res := make(ArrValue, len(args))
	for i, arg := range args {
		res[i] = New(arg)
	}
	return res
}

func (v ArrValue) Type() ValueType {
	return ArrType
}

func IsArr(v Value) bool {
	return v.Type() == ArrType
}

func (a ArrValue) Cast(t ValueType) Value {
	switch t {
	case BoolType:
		return True()
	case NumType:
		return toNum(a)
	case StrType:
		return Str(a.String())
	case ArrType:
		return a
	}
	return Null()
}

func toNum(a ArrValue) Value {
	if len(a) == 0 {
		return Num(0)
	}
	if len(a) == 1 {
		return a[0].Cast(NumType)
	}
	return Null()
}

func (a ArrValue) String() string {
	var sb strings.Builder
	for i, v := range a {
		if i > 0 {
			sb.WriteString(",")
		}
		s := v.String()
		sb.WriteString(string(s))
	}
	return sb.String()
}
