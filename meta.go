package growthbook

// VariationMeta represents meta-information that can be passed
// through to tracking callbacks.
type VariationMeta struct {
	Passthrough bool   `json:"passthrough,omitempty"`
	Key         string `json:"key,omitempty"`
	Name        string `json:"name,omitempty"`
}

func jsonVariationMeta(v interface{}, typeName string, fieldName string) *VariationMeta {
	obj, ok := v.(map[string]interface{})
	if !ok {
		logError("Invalid JSON data type", typeName, fieldName)
		return nil
	}

	passthrough := false
	key := ""
	name := ""
	vPassthrough, ptOk := obj["passthrough"]
	if ptOk {
		tmp, ok := vPassthrough.(bool)
		if !ok {
			logError("Invalid JSON data type", typeName, fieldName)
			return nil
		}
		passthrough = tmp
	}
	vKey, keyOk := obj["key"]
	if keyOk {
		tmp, ok := vKey.(string)
		if !ok {
			logError("Invalid JSON data type", typeName, fieldName)
			return nil
		}
		key = tmp
	}
	vName, nameOk := obj["name"]
	if nameOk {
		tmp, ok := vName.(string)
		if !ok {
			logError("Invalid JSON data type", typeName, fieldName)
			return nil
		}
		name = tmp
	}

	return &VariationMeta{passthrough, key, name}
}

func jsonVariationMetaArray(v interface{}, typeName string, fieldName string) ([]VariationMeta, bool) {
	vals, ok := v.([]interface{})
	if !ok {
		logError("Invalid JSON data type", typeName, fieldName)
		return nil, false
	}
	metas := make([]VariationMeta, len(vals))
	for i := range vals {
		tmp := jsonVariationMeta(vals[i], typeName, fieldName)
		if tmp == nil {
			return nil, false
		}
		metas[i] = *tmp
	}
	return metas, true
}
