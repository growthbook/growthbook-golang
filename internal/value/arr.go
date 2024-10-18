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
		return toStr(a)
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

func toStr(a ArrValue) Value {
	var sb strings.Builder
	for i, v := range a {
		if i > 0 {
			sb.WriteString(",")
		}
		sv := v.Cast(StrType)
		switch s := sv.(type) {
		case StrValue:
			sb.WriteString(string(s))
		}
	}
	return Str(sb.String())
}
