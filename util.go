package growthbook

import (
	"net/url"
	"reflect"
	"strconv"
)

// Returns an array of floats with numVariations items that are all
// equal and sum to 1.
func getEqualWeights(numVariations int) []float64 {
	if numVariations < 0 {
		numVariations = 0
	}
	equal := make([]float64, numVariations)
	for i := range equal {
		equal[i] = 1.0 / float64(numVariations)
	}
	return equal
}

// Checks if an experiment variation is being forced via a URL query
// string.
//
// As an example, if the id is "my-test" and url is
// http://localhost/?my-test=1, this function returns 1.
func getQueryStringOverride(id string, url *url.URL, numVariations int) *int {
	v, ok := url.Query()[id]
	if !ok || len(v) > 1 {
		return nil
	}

	vi, err := strconv.Atoi(v[0])
	if err != nil {
		return nil
	}

	if vi < 0 || vi >= numVariations {
		return nil
	}

	return &vi
}

// This function imitates Javascript's "truthiness" evaluation for Go
// values of unknown type.
func truthy(v interface{}) bool {
	if v == nil {
		return false
	}
	switch v.(type) {
	case string:
		return v.(string) != ""
	case bool:
		return v.(bool)
	case int:
		return v.(int) != 0
	case uint:
		return v.(uint) != 0
	case float32:
		return v.(float32) != 0
	case float64:
		return v.(float64) != 0
	}
	return true
}

// This function converts slices of concrete types to []interface{}.
// This is needed to handle the common case where a user passes an
// attribute as a []string (or []int), and this needs to be compared
// against feature data deserialized from JSON, which always results
// in []interface{} slices.
func fixSliceTypes(vin interface{}) interface{} {
	// Convert all type-specific slices to interface{} slices.
	v := reflect.ValueOf(vin)
	rv := vin
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		srv := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i).Interface()
			srv[i] = elem
		}
		rv = srv
	}
	return rv
}
