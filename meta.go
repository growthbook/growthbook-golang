package growthbook

// VariationMeta represents meta-information that can be passed
// through to tracking callbacks.
type VariationMeta struct {
	Passthrough bool
	Key         string
	Name        string
}

func jsonVariationMeta(v interface{}, typeName string, fieldName string) *VariationMeta {
	obj, ok := v.(map[string]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}

	passthrough := false
	key := ""
	name := ""
	vPassthrough, ptOk := obj["passthrough"]
	if ptOk {
		tmp, ok := vPassthrough.(bool)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		passthrough = tmp
	}
	vKey, keyOk := obj["key"]
	if keyOk {
		tmp, ok := vKey.(string)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		key = tmp
	}
	vName, nameOk := obj["name"]
	if nameOk {
		tmp, ok := vName.(string)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		name = tmp
	}

	return &VariationMeta{passthrough, key, name}
}

func jsonVariationMetaArray(v interface{}, typeName string, fieldName string) []VariationMeta {
	vals, ok := v.([]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}
	metas := make([]VariationMeta, len(vals))
	for i := range vals {
		tmp := jsonVariationMeta(vals[i], typeName, fieldName)
		if tmp == nil {
			return nil
		}
		metas[i] = *tmp
	}
	return metas
}
