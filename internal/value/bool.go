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
	default:
		return Null()
	}
}

func IsBool(v Value) bool {
	return v.Type() == BoolType
}
