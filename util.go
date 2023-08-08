package growthbook

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
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

func decrypt(encrypted string, encKey string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(encKey)
	if err != nil {
		return "", err
	}

	splits := strings.Split(encrypted, ".")
	if len(splits) != 2 {
		return "", errors.New("invalid format for key")
	}

	iv, err := base64.StdEncoding.DecodeString(splits[0])
	if err != nil {
		return "", err
	}

	cipherText, err := base64.StdEncoding.DecodeString(splits[1])
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	if len(iv) != block.BlockSize() {
		return "", errors.New("invalid IV length")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(cipherText, cipherText)

	cipherText, err = unpad(cipherText)
	if err != nil {
		return "", err
	}

	return string(cipherText), nil
}

// Remove PKCS #7 padding.

func unpad(buf []byte) ([]byte, error) {
	bufLen := len(buf)
	if bufLen == 0 {
		return nil, errors.New("crypto/padding: invalid padding size")
	}

	pad := buf[bufLen-1]
	if pad == 0 {
		return nil, errors.New("crypto/padding: invalid last byte of padding")
	}

	padLen := int(pad)
	if padLen > bufLen || padLen > 16 {
		return nil, errors.New("crypto/padding: invalid padding size")
	}

	for _, v := range buf[bufLen-padLen : bufLen-1] {
		if v != pad {
			return nil, errors.New("crypto/padding: invalid padding")
		}
	}

	return buf[:bufLen-padLen], nil
}

// This function imitates Javascript's "truthiness" evaluation for Go
// values of unknown type.
func truthy(v any) bool {
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

// This function converts slices of concrete types to []any.
// This is needed to handle the common case where a user passes an
// attribute as a []string (or []int), and this needs to be compared
// against feature data deserialized from JSON, which always results
// in []any slices.
func fixSliceTypes(vin any) any {
	// Convert all type-specific slices to any slices.
	v := reflect.ValueOf(vin)
	rv := vin
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		srv := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i).Interface()
			srv[i] = elem
		}
		rv = srv
	}
	return rv
}

func isURLTargeted(url *url.URL, targets []URLTarget) (bool, error) {
	if len(targets) == 0 {
		return false, nil
	}

	hasIncludeRules := false
	isIncluded := false

	for _, t := range targets {
		match, err := evalURLTarget(url, t.Type, t.Pattern)
		if err != nil {
			return false, err
		}
		if !t.Include {
			if match {
				return false, nil
			}
		} else {
			hasIncludeRules = true
			if match {
				isIncluded = true
			}
		}
	}

	return isIncluded || !hasIncludeRules, nil
}

func evalURLTarget(url *url.URL, typ URLTargetType, pattern string) (bool, error) {
	if typ == RegexURLTarget {
		regex, err := getURLRegexp(pattern)
		if err != nil {
			return false, err
		}
		if regex == nil {
			return false, nil
		}
		return regex.MatchString(url.String()) || regex.MatchString(url.Path), nil
	} else if typ == SimpleURLTarget {
		return evalSimpleURLTarget(url, pattern)
	}
	return false, nil
}

type comp struct {
	actual   string
	expected string
	isPath   bool
}

func evalSimpleURLTarget(actual *url.URL, pattern string) (bool, error) {
	// If a protocol is missing, but a host is specified, add `https://`
	// to the front. Use "_____" as the wildcard since `*` is not a valid
	// hostname in some browsers
	schemeRe := regexp.MustCompile(`(?i)^([^:/?]*)\.`)
	pattern = schemeRe.ReplaceAllString(pattern, "https://$1.")
	wildcardRe := regexp.MustCompile(`\*`)
	pattern = wildcardRe.ReplaceAllLiteralString(pattern, "_____")
	expected, err := url.Parse(pattern)
	if err != nil {
		return false, errors.New("Failed to parse URL pattern: " + pattern)
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
		return false, errors.New("Failed to parse actual URL query parameters: " + actual.RawQuery)
	}
	expectedParams, err := url.ParseQuery(expected.RawQuery)
	if err != nil {
		return false, errors.New("Failed to parse expected URL query parameters: " + expected.RawQuery)
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
		eval, err := evalSimpleURLPart(comp.actual, comp.expected, comp.isPath)
		if err != nil {
			return false, err
		}
		if !eval {
			return false, nil
		}
	}
	return true, nil
}

func evalSimpleURLPart(actual string, pattern string, isPath bool) (bool, error) {
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
		return false, errors.New("Failed to compile regexp: " + escaped)
	}
	return regex.MatchString(actual), nil
}

func getURLRegexp(regexString string) (*regexp.Regexp, error) {
	retval, err := regexp.Compile(regexString)
	if err == nil {
		return retval, nil
	}
	return nil, errors.New("Failed to compile URL regexp:" + err.Error())
}

var (
	versionStripRe = regexp.MustCompile(`(^v|\+.*$)`)
	versionSplitRe = regexp.MustCompile(`[-.]`)
	versionNumRe   = regexp.MustCompile(`^[0-9]+$`)
)

func paddedVersionString(input string) string {
	// Remove build info and leading `v` if any
	// Split version into parts (both core version numbers and pre-release tags)
	// "v1.2.3-rc.1+build123" -> ["1","2","3","rc","1"]
	stripped := versionStripRe.ReplaceAllLiteralString(input, "")
	parts := versionSplitRe.Split(stripped, -1)

	// If it's SemVer without a pre-release, add `~` to the end
	// ["1","0","0"] -> ["1","0","0","~"]
	// "~" is the largest ASCII character, so this will make "1.0.0"
	// greater than "1.0.0-beta" for example
	if len(parts) == 3 {
		parts = append(parts, "~")
	}

	// Left pad each numeric part with spaces so string comparisons will
	// work ("9">"10", but " 9"<"10")
	for i := range parts {
		if versionNumRe.MatchString(parts[i]) {
			parts[i] = strings.Repeat(" ", 5-len(parts[i])) + parts[i]
		}
	}
	// Then, join back together into a single string
	return strings.Join(parts, "-")
}
