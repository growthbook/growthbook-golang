package value

import "strconv"

type StrValue string

func Str(s string) StrValue {
	return StrValue(s)
}

func (s StrValue) Type() ValueType {
	return StrType
}

func (s StrValue) Cast(t ValueType) Value {
	switch t {
	case NumType:
		f, err := strconv.ParseFloat(string(s), 64)
		if err != nil {
			return Null()
		}
		return Num(f)
	case StrType:
		return s
	case BoolType:
		return Bool(s != "")
	default:
		return Null()
	}
}

func IsStr(v Value) bool {
	return v.Type() == StrType
}
