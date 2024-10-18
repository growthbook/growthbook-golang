package value

//import "strconv"

type BoolValue bool

func Bool(b bool) BoolValue {
	return BoolValue(b)
}

func True() BoolValue {
	return BoolValue(true)
}

func False() BoolValue {
	return BoolValue(false)
}

func (v BoolValue) Type() ValueType {
	return BoolType
}

func (v BoolValue) Cast(t ValueType) Value {
	switch t {
	case BoolType:
		return v
	case NumType:
		if v == True() {
			return Num(1)
		} else {
			return Num(0)
		}
	case StrType:
		if v == True() {
			return Str("true")
		} else {
			return Str("false")
		}
	default:
		return Null()
	}
}

func IsBool(v Value) bool {
	return v.Type() == BoolType
}
