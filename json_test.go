package growthbook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"reflect"
	"testing"
)

// Main test function for running JSON-based tests. These all use a
// jsonTest helper function to read and parse the JSON test case file.

func TestJSON(t *testing.T) {
	SetLogger(&testLog)

	jsonTest(t, "evalCondition", jsonTestEvalCondition)
	jsonMapTest(t, "versionCompare", jsonTestVersionCompare)
	jsonTest(t, "hash", jsonTestHash)
	jsonTest(t, "getBucketRange", jsonTestGetBucketRange)
	jsonTest(t, "feature", jsonTestFeature)
	jsonTest(t, "run", jsonTestRun)
	jsonTest(t, "chooseVariation", jsonTestChooseVariation)
	jsonTest(t, "getQueryStringOverride", jsonTestQueryStringOverride)
	jsonTest(t, "inNamespace", jsonTestInNamespace)
	jsonTest(t, "getEqualWeights", jsonTestGetEqualWeights)
	jsonTest(t, "decrypt", jsonTestDecrypt)
}

// Test functions driven from JSON cases. Each of this has a similar
// structure, first extracting test data from the JSON data into typed
// values, then performing the test.

// Condition evaluation tests.
//
// Test parameters: name, condition, attributes, result
func jsonTestEvalCondition(t *testing.T, test []byte) {
	var name string
	var condition *Condition
	var value map[string]interface{}
	var expected bool
	unmarshalTest(test, []interface{}{&name, &condition, &value, &expected})

	attrs := Attributes(value)
	result := condition.Eval(attrs)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("unexpected result: %v", result)
	}
}

// Version comparison tests.
//
// Test parameters: ...
func jsonTestVersionCompare(t *testing.T, comparison string, test []byte) {
	var v1 string
	var v2 string
	var expected bool
	unmarshalTest(test, []interface{}{&v1, &v2, &expected})

	pv1 := paddedVersionString(v1)
	pv2 := paddedVersionString(v2)

	switch comparison {
	case "eq":
		if (pv1 == pv2) != expected {
			t.Errorf("unexpected result: '%s' eq '%s' => %v", v1, v2, pv1 == pv2)
		}
	case "gt":
		if (pv1 > pv2) != expected {
			t.Errorf("unexpected result: '%s' gt '%s' => %v", v1, v2, pv1 == pv2)
		}
	case "lt":
		if (pv1 < pv2) != expected {
			t.Errorf("unexpected result: '%s' lt '%s' => %v", v1, v2, pv1 == pv2)
		}
	}
}

// Hash function tests.
//
// Test parameters: value, hash
func jsonTestHash(t *testing.T, test []byte) {
	var seed string
	var value string
	var version int
	var expected *float64
	unmarshalTest(test, []interface{}{&seed, &value, &version, &expected})

	result := hash(seed, value, version)
	if expected == nil {
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	} else {
		if result == nil {
			t.Errorf("expected non-nil result, got nil")
		}
		if !reflect.DeepEqual(*result, *expected) {
			t.Errorf("unexpected result: %v", *result)
		}
	}
}

// Bucket range tests.
//
// Test parameters: name, args ([numVariations, coverage, weights]), result
func jsonTestGetBucketRange(t *testing.T, test []byte) {
	var name string
	var args json.RawMessage
	var result [][]float64
	unmarshalTest(test, []interface{}{&name, &args, &result})

	var numVariations int
	var coverage float64
	var weights []float64
	unmarshalTest(args, []interface{}{&numVariations, &coverage, &weights})

	variations := make([]Range, len(result))
	for i, v := range result {
		variations[i] = Range{v[0], v[1]}
	}

	ranges := roundRanges(getBucketRanges(numVariations, coverage, weights))

	if !reflect.DeepEqual(ranges, variations) {
		t.Errorf("unexpected value: %v", result)
	}

	// Handle expected warnings.
	if coverage < 0 || coverage > 1 {
		if len(testLog.errors) != 0 && len(testLog.warnings) != 1 {
			t.Errorf("expected coverage log warning")
		}
		testLog.reset()
	}
	totalWeights := 0.0
	for _, w := range weights {
		totalWeights += w
	}
	if totalWeights != 1 {
		if len(testLog.errors) != 0 && len(testLog.warnings) != 1 {
			t.Errorf("expected weight sum log warning")
		}
		testLog.reset()
	}
	if len(weights) != len(result) {
		if len(testLog.errors) != 0 && len(testLog.warnings) != 1 {
			t.Errorf("expected weight length log warning")
		}
		testLog.reset()
	}
}

// Feature tests.
//
// Test parameters: name, context, feature key, result
func jsonTestFeature(t *testing.T, test []byte) {
	var name string
	var context *testContext
	var featureKey string
	var expected FeatureResult
	unmarshalTest(test, []interface{}{&name, &context, &featureKey, &expected})
	client := NewClient(nil).
		WithFeatures(context.Features).
		WithForcedVariations(context.ForcedVariations)

	retval := client.EvalFeature(featureKey, context.Attributes)

	if !reflect.DeepEqual(retval, &expected) {
		t.Errorf("unexpected value: %v", retval)
	}

	expectedWarnings := map[string]int{
		"unknown feature key": 1,
		"ignores empty rules": 1,
	}
	handleExpectedWarnings(t, name, expectedWarnings)
}

// Experiment tests.
//
// Test parameters: name, context, experiment, value, inExperiment
func jsonTestRun(t *testing.T, test []byte) {
	var name string
	var context *testContext
	var experiment *Experiment
	var result interface{}
	var inExperiment bool
	var hashUsed bool
	unmarshalTest(test, []interface{}{&name, &context, &experiment, &result, &inExperiment, &hashUsed})

	opt := Options{Disabled: !context.Enabled, QAMode: context.QAMode}
	if context.URL != "" {
		url, err := url.Parse(context.URL)
		if err != nil {
			t.Errorf("invalid URL")
		}
		opt.URL = url
	}
	client := NewClient(&opt).
		WithFeatures(context.Features).
		WithForcedVariations(context.ForcedVariations)
	r := client.Run(experiment, context.Attributes)

	if !reflect.DeepEqual(r.Value, result) {
		t.Errorf("unexpected result value: %v (should be %v)", r.Value, result)
	}
	if r.InExperiment != inExperiment {
		t.Errorf("unexpected inExperiment value: %v", r.InExperiment)
	}
	if r.HashUsed != hashUsed {
		t.Errorf("unexpected hashUsed value: %v", r.HashUsed)
	}

	expectedWarnings := map[string]int{
		"single variation": 1,
	}
	handleExpectedWarnings(t, name, expectedWarnings)
}

// Variation choice tests.
//
// Test parameters: name, hash, ranges, result
func jsonTestChooseVariation(t *testing.T, test []byte) {
	var name string
	var hash float64
	var ranges [][]float64
	var result int
	unmarshalTest(test, []interface{}{&name, &hash, &ranges, &result})

	variations := make([]Range, len(ranges))
	for i, v := range ranges {
		variations[i] = Range{v[0], v[1]}
	}

	variation := chooseVariation(hash, variations)
	if variation != int(result) {
		t.Errorf("unexpected result: %d", variation)
	}
}

// Query string override tests
//
// Test parameters: name, experiment key, url, numVariations, result
func jsonTestQueryStringOverride(t *testing.T, test []byte) {
	var name string
	var key string
	var rawURL string
	var numVariations int
	var expected *int
	unmarshalTest(test, []interface{}{&name, &key, &rawURL, &numVariations, &expected})
	url, err := url.Parse(rawURL)
	if err != nil {
		log.Fatal("invalid URL")
	}

	override := getQueryStringOverride(key, url, numVariations)
	if !reflect.DeepEqual(override, expected) {
		t.Errorf("unexpected result: %v", override)
	}
}

// Namespace inclusion tests
//
// Test parameters: name, id, namespace, result

func jsonTestInNamespace(t *testing.T, test []byte) {
	var name string
	var id string
	var namespace *Namespace
	var expected bool
	unmarshalTest(test, []interface{}{&name, &id, &namespace, &expected})

	result := namespace.inNamespace(id)
	if result != expected {
		t.Errorf("unexpected result: %v", result)
	}
}

// Equal weight calculation tests.
//
// Test parameters: numVariations, result
func jsonTestGetEqualWeights(t *testing.T, test []byte) {
	var numVariations int
	var expected []float64
	unmarshalTest(test, []interface{}{&numVariations, &expected})

	result := getEqualWeights(numVariations)
	if !reflect.DeepEqual(round(result), round(expected)) {
		t.Errorf("unexpected value: %v", result)
	}
}

// Decryption function tests.
//
// Test parameters: name, encryptedString, key, expected
func jsonTestDecrypt(t *testing.T, test []byte) {
	var name string
	var encrypted string
	var key string
	var expected *string
	unmarshalTest(test, []interface{}{&name, &encrypted, &key, &expected})

	result, err := decrypt(encrypted, key)
	if expected == nil {
		if err == nil {
			t.Errorf("expected error return")
		}
	} else {
		if err != nil {
			t.Errorf("error in decrypt: %v", err)
		} else if !reflect.DeepEqual(result, *expected) {
			t.Errorf("unexpected result: %v", result)
			fmt.Printf("expected: '%s' (%d)\n", *expected, len(*expected))
			fmt.Println([]byte(*expected))
			fmt.Printf("     got: '%s' (%d)\n", result, len(result))
			fmt.Println([]byte(result))
		}
	}
}

//------------------------------------------------------------------------------
//
//  TEST UTILITIES
//

// Run a set of JSON test cases provided as a JSON array.

func jsonTest(t *testing.T, label string,
	fn func(t *testing.T, test []byte)) {
	content, err := ioutil.ReadFile("cases.json")
	if err != nil {
		log.Fatal(err)
	}

	// Unmarshal all test cases at once.
	allCases := map[string]interface{}{}
	err = json.Unmarshal(content, &allCases)
	if err != nil {
		log.Fatal(err)
	}

	// Extract just the test cases for the test type we're working on.
	cases := allCases[label].([]interface{})

	// Extract the test data for each case as a JSON array and pass to
	// the test function.
	t.Run("json test suite: "+label, func(t *testing.T) {
		// Run tests one at a time: each test's JSON data is an array,
		// with the interpretation of the array entries depending on the
		// test type.
		for itest, gtest := range cases {
			test, ok := gtest.([]interface{})
			if !ok {
				log.Fatal("unpacking JSON test data")
			}
			name, ok := test[0].(string)
			if !ok {
				name = ""
			}
			t.Run(fmt.Sprintf("[%d] %s", itest, name), func(t *testing.T) {
				// Handle logging during tests: reset log before each test,
				// make sure there are no errors or warnings (some tests that
				// check for correct handling of out-of-range parameters
				// trigger warnings, but these are handled within the test
				// themselves).
				testLog.reset()
				jsonTest, err := json.Marshal(test)
				if err != nil {
					t.Errorf("CAN'T CONVERT TEST BACK TO JSON!")
				}
				fn(t, jsonTest)
				if len(testLog.errors) != 0 {
					t.Errorf("test log has errors: %s", testLog.allErrors())
				}
				if len(testLog.warnings) != 0 {
					t.Errorf("test log has warnings: %s", testLog.allWarnings())
				}
			})
		}
	})
}

// Run a set of JSON test cases provided as a JSON map.

func jsonMapTest(t *testing.T, label string,
	fn func(t *testing.T, label string, test []byte)) {
	content, err := ioutil.ReadFile("cases.json")
	if err != nil {
		log.Fatal(err)
	}

	// Unmarshal all test cases at once.
	allCases := map[string]interface{}{}
	err = json.Unmarshal(content, &allCases)
	if err != nil {
		log.Fatal(err)
	}

	// Extract just the test cases for the test type we're working on.
	cases := allCases[label].(map[string]interface{})

	// Extract the test data for each case as a JSON array and pass to
	// the test function.
	t.Run("json test suite: "+label, func(t *testing.T) {
		// Run tests one at a time: each test's JSON data is an array,
		// keyed by a string label, and the interpretation of the array
		// entries depends on the test type.
		itest := 1
		for name, gtest := range cases {
			tests, ok := gtest.([]interface{})
			if !ok {
				log.Fatal("unpacking JSON test data")
			}

			t.Run(fmt.Sprintf("[%d] %s", itest, name), func(t *testing.T) {
				// Handle logging during tests: reset log before each test,
				// make sure there are no errors or warnings (some tests that
				// check for correct handling of out-of-range parameters
				// trigger warnings, but these are handled within the test
				// themselves).
				testLog.reset()
				for _, test := range tests {
					jsonTest, err := json.Marshal(test)
					if err != nil {
						t.Errorf("CAN'T CONVERT TEST BACK TO JSON!")
					}
					fn(t, name, jsonTest)
				}
				if len(testLog.errors) != 0 {
					t.Errorf("test log has errors: %s", testLog.allErrors())
				}
				if len(testLog.warnings) != 0 {
					t.Errorf("test log has warnings: %s", testLog.allWarnings())
				}
			})
			itest++
		}
	})
}

type testContext struct {
	Enabled          bool                `json:"enabled"`
	URL              string              `json:"url"`
	QAMode           bool                `json:"qaMode"`
	Attributes       Attributes          `json:"attributes"`
	Features         FeatureMap          `json:"features"`
	ForcedVariations ForcedVariationsMap `json:"forcedVariations"`
}

func (ctx *testContext) UnmarshalJSON(data []byte) error {
	type alias testContext
	val := alias{Enabled: true}
	err := json.Unmarshal(data, &val)
	if err != nil {
		return err
	}
	*ctx = testContext(val)
	return nil
}

func unmarshalTest(in []byte, d []interface{}) {
	okLen := len(d)
	if err := json.Unmarshal(in, &d); err != nil {
		log.Fatal("unpacking test data:", err)
	}
	if len(d) != okLen {
		log.Fatal("unpacking test data: wrong length")
	}
}
