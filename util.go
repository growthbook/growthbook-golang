package growthbook

import (
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
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

func isURLTargeted(url *url.URL, targets []URLTarget) bool {
	if len(targets) == 0 {
		return false
	}

	hasIncludeRules := false
	isIncluded := false

	for _, t := range targets {
		match := evalURLTarget(url, t.Type, t.Pattern)
		if !t.Include {
			if match {
				return false
			}
		} else {
			hasIncludeRules = true
			if match {
				isIncluded = true
			}
		}
	}

	return isIncluded || !hasIncludeRules
}

func evalURLTarget(url *url.URL, typ URLTargetType, pattern string) bool {
	if typ == RegexURLTarget {
		regex := getUrlRegExp(pattern)
		if regex == nil {
			return false
		}
		return regex.MatchString(url.String()) || regex.MatchString(url.Path)
	} else if typ == SimpleURLTarget {
		return evalSimpleUrlTarget(url, pattern)
	}
	return false
}

type comp struct {
	actual   string
	expected string
	isPath   bool
}

func evalSimpleUrlTarget(actual *url.URL, pattern string) bool {
	// If a protocol is missing, but a host is specified, add `https://`
	// to the front. Use "_____" as the wildcard since `*` is not a valid
	// hostname in some browsers
	schemeRe := regexp.MustCompile(`(?i)^([^:/?]*)\.`)
	pattern = schemeRe.ReplaceAllString(pattern, "https://$1.")
	wildcardRe := regexp.MustCompile(`\*`)
	pattern = wildcardRe.ReplaceAllLiteralString(pattern, "_____")
	expected, err := url.Parse(pattern)
	if err != nil {
		logError("Failed to parse URL pattern: ", pattern)
		return false
	}
	if expected.Host == "" {
		expected.Host = "_____"
	}

	// Compare each part of the URL separately
	comps := []comp{
		{actual.Host, expected.Host, false},
		{actual.Path, expected.Path, true},
	}
	// We only want to compare hashes if it's explicitly being targeted
	if expected.Fragment != "" {
		comps = append(comps, comp{actual.Fragment, expected.Fragment, false})
	}

	actualParams, err := url.ParseQuery(actual.RawQuery)
	if err != nil {
		logError("Failed to parse actual URL query parameters: ", actual.RawQuery)
		return false
	}
	expectedParams, err := url.ParseQuery(expected.RawQuery)
	if err != nil {
		logError("Failed to parse expected URL query parameters: ", expected.RawQuery)
		return false
	}
	for param, expectedValue := range expectedParams {
		actualValue := ""
		if actualParams.Has(param) {
			actualValue = actualParams[param][0]
		}
		comps = append(comps, comp{actualValue, expectedValue[0], false})
	}

	// If any comparisons fail, the whole thing fails
	for _, comp := range comps {
		if !evalSimpleUrlPart(comp.actual, comp.expected, comp.isPath) {
			return false
		}
	}
	return true
}

func evalSimpleUrlPart(actual string, pattern string, isPath bool) bool {
	// Escape special regex characters.
	specialRe := regexp.MustCompile(`([*.+?^${}()|[\]\\])`)
	escaped := specialRe.ReplaceAllString(pattern, "\\$1")
	escaped = strings.Replace(escaped, "_____", ".*", -1)

	if isPath {
		// When matching pathname, make leading/trailing slashes optional
		slashRe := regexp.MustCompile(`(^\/|\/$)`)
		escaped = slashRe.ReplaceAllLiteralString(escaped, "")
		escaped = "\\/?" + escaped + "\\/?"
	}

	escaped = "(?i)^" + escaped + "$"
	regex, err := regexp.Compile(escaped)
	if err != nil {
		logError("Failed to compile regexp: ", escaped)
		return false
	}
	return regex.MatchString(actual)
}

func getUrlRegExp(regexString string) *regexp.Regexp {
	retval, err := regexp.Compile(regexString)
	if err == nil {
		return retval
	}
	logError("Failed to compile URL regexp:", err)
	return nil
}
