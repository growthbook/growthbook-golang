package value

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

func (_ ArrValue) Cast(t ValueType) Value {
	switch t {
	case BoolType:
		return True()
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
