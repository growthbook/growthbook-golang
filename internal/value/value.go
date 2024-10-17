package value

import "reflect"

// Value represents Grothbok's internal set of allowed values.
// Both in rules/conditions and attributes.
// They follow JS behaviour for casting, as our calculations should result
// into exact same values as main JS Growthbook SDKs.
type Value interface {
	// Just to simplify type switches.
	Type() ValueType
	// Cast to other types, similar to JS
	Cast(ValueType) Value
}

type ValueType int

const (
	NullType ValueType = iota
	BoolType
	NumType
	StrType
	ArrType
	ObjType
)

func New(a any) Value {
	if a == nil {
		return Null()
	}
	switch v := a.(type) {
	case Value:
		return v
	default:
		return fromAny(a)
	}
}

func fromAny(a any) Value {
	ref := reflect.ValueOf(a)
	switch {
	case ref.CanFloat():
		return Num(ref.Float())
	case ref.CanInt():
		return Num(ref.Int())
	case ref.CanUint():
		return Num(ref.Uint())
	case ref.Kind() == reflect.Bool:
		return Bool(ref.Bool())
	case ref.Kind() == reflect.String:
		return Str(ref.String())
	default:
		return Null()
	}
}
