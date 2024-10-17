package value

type ObjValue map[string]Value

func Obj(args map[string]any) ObjValue {
	res := make(ObjValue, len(args))
	for k, v := range args {
		res[k] = New(v)
	}
	return res
}

func (o ObjValue) Type() ValueType {
	return ObjType
}

func IsObj(v Value) bool {
	return v.Type() == ObjType
}

func (o ObjValue) Cast(t ValueType) Value {
	return Null()
}
