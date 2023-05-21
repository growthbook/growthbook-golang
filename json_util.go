package growthbook

func jsonString(v interface{}, typeName string, fieldName string) string {
	tmp, ok := v.(string)
	if ok {
		return tmp
	}
	logError("Invalid JSON data type", typeName, fieldName)
	return ""
}

func jsonBool(v interface{}, typeName string, fieldName string) bool {
	tmp, ok := v.(bool)
	if ok {
		return tmp
	}
	logError("Invalid JSON data type", typeName, fieldName)
	return false
}

func jsonInt(v interface{}, typeName string, fieldName string) int {
	tmp, ok := v.(float64)
	if ok {
		return int(tmp)
	}
	logError("Invalid JSON data type", typeName, fieldName)
	return 0
}

func jsonFloat(v interface{}, typeName string, fieldName string) float64 {
	tmp, ok := v.(float64)
	if ok {
		return tmp
	}
	logError("Invalid JSON data type", typeName, fieldName)
	return 0.0
}

func jsonMaybeFloat(v interface{}, typeName string, fieldName string) *float64 {
	tmp, ok := v.(float64)
	if ok {
		return &tmp
	}
	logError("Invalid JSON data type", typeName, fieldName)
	return nil
}

func jsonFloatArray(v interface{}, typeName string, fieldName string) []float64 {
	vals, ok := v.([]interface{})
	if !ok {
		logError("Invalid JSON data type", typeName, fieldName)
		return nil
	}
	fvals := make([]float64, len(vals))
	for i := range vals {
		tmp, ok := vals[i].(float64)
		if !ok {
			logError("Invalid JSON data type", typeName, fieldName)
			return nil
		}
		fvals[i] = tmp
	}
	return fvals
}
