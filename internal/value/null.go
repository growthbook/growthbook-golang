package value

type NullValue struct{}

func Null() Value {
	return NullValue{}
}

func (n NullValue) Type() ValueType {
	return NullType
}

func (n NullValue) Cast(t ValueType) Value {
	switch t {
	case BoolType:
		return False()
	case NumType:
		return Num(0)
	case StrType:
		return Str(n.String())
	default:
		return Null()
	}
}

func IsNull(v Value) bool {
	return v.Type() == NullType
}

func (n NullValue) String() string {
	return "null"
}
