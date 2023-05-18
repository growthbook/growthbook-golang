package growthbook

// Filter represents a filter condition for experiment mutual
// exclusion.
type Filter struct {
	Attribute   string
	Seed        string
	HashVersion int
	Ranges      []Range
}

func jsonFilter(v interface{}, typeName string, fieldName string) *Filter {
	obj, ok := v.(map[string]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}

	attribute := ""
	seed := ""
	hashVersion := 0
	var ranges []Range
	vAttribute, atOk := obj["attribute"]
	if atOk {
		tmp, ok := vAttribute.(string)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		attribute = tmp
	}
	vSeed, seedOk := obj["seed"]
	if seedOk {
		tmp, ok := vSeed.(string)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		seed = tmp
	}
	vHashVersion, hvOk := obj["hashVersion"]
	if hvOk {
		tmp, ok := vHashVersion.(float64)
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		vHashVersion = int(tmp)
	}
	vRanges, rngOk := obj["ranges"]
	if rngOk {
		tmp, ok := vRanges.([]interface{})
		if !ok {
			logError(ErrJSONInvalidType, typeName, fieldName)
			return nil
		}
		ranges = jsonRangeArray(tmp, typeName, fieldName)
	}

	return &Filter{attribute, seed, hashVersion, ranges}
}

func jsonFilterArray(v interface{}, typeName string, fieldName string) []Filter {
	vals, ok := v.([]interface{})
	if !ok {
		logError(ErrJSONInvalidType, typeName, fieldName)
		return nil
	}
	filters := make([]Filter, len(vals))
	for i := range vals {
		tmp := jsonFilter(vals[i], typeName, fieldName)
		if tmp == nil {
			return nil
		}
		filters[i] = *tmp
	}
	return filters
}
