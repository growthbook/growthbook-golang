package value

import (
	"strconv"
)

type NumValue float64

type number interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 |
		float32 | float64
}

func Num[T number](n T) Value {
	return NumValue(n)
}

func (n NumValue) Type() ValueType {
	return NumType
}

func (n NumValue) Cast(t ValueType) Value {
	switch t {
	case NumType:
		return n
	case BoolType:
		return Bool(n != 0)
	case StrType:
		return Str(n.String())
	default:
		return Null()
	}
}

func IsNum(v Value) bool {
	return v.Type() == NumType
}

func (n NumValue) String() string {
	return strconv.FormatFloat(float64(n), 'f', -1, 64)
}
