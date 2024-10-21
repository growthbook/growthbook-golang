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
	switch t {
	case BoolType:
		return True()
	case ObjType:
		return o
	}
	return Null()
}

func (o ObjValue) Path(path ...string) Value {
	var cur ObjValue = o
	for _, field := range path {
		val, ok := cur[field]
		if !ok {
			return Null()
		}
		cur, ok = val.(ObjValue)
		if !ok {
			return val
		}
	}
	return cur
}
