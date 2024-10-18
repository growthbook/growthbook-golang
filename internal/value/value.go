package value

import "reflect"

// Value represents Grothbok's internal set of allowed values.
// Both in rules/conditions and attributes.
// They follow JS behaviour for casting, as our calculations should result
// into exact same values as main JS Growthbook SDKs.
type Value interface {
	//Type results to ValueType enum
	Type() ValueType
	// Cast to other types, similarly to JS
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

func Equal(v1, v2 Value) bool {
	if v1.Type() != v2.Type() {
		return false
	}
	switch v1.Type() {
	case ArrType:
		a1, a2 := v1.(ArrValue), v2.(ArrValue)
		if len(a1) != len(a2) {
			return false
		}
		for i, v := range a1 {
			if !Equal(v, a2[i]) {
				return false
			}
		}
		return true
	case ObjType:
		o1, o2 := v1.(ObjValue), v2.(ObjValue)
		if len(o1) != len(o2) {
			return false
		}
		for k, v := range o1 {
			if !Equal(v, o2[k]) {
				return false
			}
		}
		return true
	default:
		return v1 == v2
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
