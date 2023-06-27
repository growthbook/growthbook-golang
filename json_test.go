package growthbook

import (
	"encoding/json"
	"errors"
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
func jsonTestEvalCondition(t *testing.T, test []interface{}) {
	condition, ok1 := test[1].(map[string]interface{})
	value, ok2 := test[2].(map[string]interface{})
	expected, ok3 := test[3].(bool)
	if !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	cond := BuildCondition(condition)
	if cond == nil {
		log.Fatal(errors.New("failed to build condition"))
	}
	attrs := Attributes(value)
	result := cond.Eval(attrs)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("unexpected result: %v", result)
	}
}

// Version comparison tests.
//
// Test parameters: ...
func jsonTestVersionCompare(t *testing.T, comparison string, test []interface{}) {
	for _, oneTest := range test {
		testData, ok := oneTest.([]interface{})
		if !ok || len(testData) != 3 {
			log.Fatal("unpacking test data")
		}
		v1, ok1 := testData[0].(string)
		v2, ok2 := testData[1].(string)
		expected, ok3 := testData[2].(bool)
		if !ok1 || !ok2 || !ok3 {
			log.Fatal("unpacking test data")
		}

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
}

// Hash function tests.
//
// Test parameters: value, hash
func jsonTestHash(t *testing.T, test []interface{}) {
	seed, ok0 := test[0].(string)
	value, ok1 := test[1].(string)
	version, ok2 := test[2].(float64)
	expectedValue, ok3 := test[3].(float64)
	var expected *float64
	if ok3 {
		expected = &expectedValue
	} else {
		ok3 = test[3] == nil
	}
	if !ok0 || !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	result := hash(seed, value, int(version))
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
func jsonTestGetBucketRange(t *testing.T, test []interface{}) {
	args, ok1 := test[1].([]interface{})
	result, ok2 := test[2].([]interface{})
	if !ok1 || !ok2 {
		log.Fatal("unpacking test data")
	}

	numVariations, argok0 := args[0].(float64)
	coverage, argok1 := args[1].(float64)
	if !argok0 || !argok1 {
		log.Fatal("unpacking test data")
	}
	var weights []float64
	totalWeights := 0.0
	if args[2] != nil {
		wgts, ok := args[2].([]interface{})
		if !ok {
			log.Fatal("unpacking test data")
		}
		weights = make([]float64, len(wgts))
		for i, w := range wgts {
			weights[i] = w.(float64)
			totalWeights += w.(float64)
		}
	}

	variations := make([]Range, len(result))
	for i, v := range result {
		vr, ok := v.([]interface{})
		if !ok || len(vr) != 2 {
			log.Fatal("unpacking test data")
		}
		variations[i] = Range{vr[0].(float64), vr[1].(float64)}
	}

	ranges := roundRanges(getBucketRanges(int(numVariations), coverage, weights))

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
func jsonTestFeature(t *testing.T, test []interface{}) {
	contextDict, ok1 := test[1].(map[string]interface{})
	featureKey, ok2 := test[2].(string)
	expectedDict, ok3 := test[3].(map[string]interface{})
	if !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	context := BuildContext(contextDict)
	growthbook := New(context)
	expected := BuildFeatureResult(expectedDict)
	if expected == nil {
		t.Errorf("unexpected nil from BuildFeatureResult")
	}
	retval := growthbook.Feature(featureKey)

	// fmt.Println("== RESULT ======================================================================")
	// fmt.Println(retval)
	// fmt.Println(retval.Experiment)
	// fmt.Println(retval.ExperimentResult)
	// fmt.Println("--------------------------------------------------------------------------------")
	// fmt.Println(expected)
	// fmt.Println(expected.Experiment)
	// fmt.Println(expected.ExperimentResult)
	// fmt.Println("== EXPECTED ====================================================================")

	if !reflect.DeepEqual(retval, expected) {
		t.Errorf("unexpected value: %v", retval)
	}

	expectedWarnings := map[string]int{
		"unknown feature key": 1,
		"ignores empty rules": 1,
	}
	handleExpectedWarnings(t, test, expectedWarnings)
}

// Experiment tests.
//
// Test parameters: name, context, experiment, value, inExperiment
func jsonTestRun(t *testing.T, test []interface{}) {
	contextDict, ok1 := test[1].(map[string]interface{})
	experimentDict, ok2 := test[2].(map[string]interface{})
	resultValue := test[3]
	resultInExperiment, ok3 := test[4].(bool)
	if !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	context := BuildContext(contextDict)
	growthbook := New(context)
	experiment := BuildExperiment(experimentDict)
	if experiment == nil {
		t.Errorf("unexpected nil from BuildExperiment")
	}
	result := growthbook.Run(experiment)

	if !reflect.DeepEqual(result.Value, resultValue) {
		t.Errorf("unexpected result value: %v", result.Value)
	}
	if result.InExperiment != resultInExperiment {
		t.Errorf("unexpected inExperiment value: %v", result.InExperiment)
	}

	expectedWarnings := map[string]int{
		"single variation": 1,
	}
	handleExpectedWarnings(t, test, expectedWarnings)
}

// Variation choice tests.
//
// Test parameters: name, hash, ranges, result
func jsonTestChooseVariation(t *testing.T, test []interface{}) {
	hash, ok1 := test[1].(float64)
	ranges, ok2 := test[2].([]interface{})
	result, ok3 := test[3].(float64)
	if !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	variations := make([]Range, len(ranges))
	for i, v := range ranges {
		vr, ok := v.([]interface{})
		if !ok || len(vr) != 2 {
			log.Fatal("unpacking test data")
		}
		variations[i] = Range{vr[0].(float64), vr[1].(float64)}
	}

	variation := chooseVariation(hash, variations)
	if variation != int(result) {
		t.Errorf("unexpected result: %d", variation)
	}
}

// Query string override tests
//
// Test parameters: name, experiment key, url, numVariations, result
func jsonTestQueryStringOverride(t *testing.T, test []interface{}) {
	key, ok1 := test[1].(string)
	rawURL, ok2 := test[2].(string)
	numVariations, ok3 := test[3].(float64)
	result := test[4]
	var expected *int
	if result != nil {
		tmp := int(result.(float64))
		expected = &tmp
	}
	if !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}
	url, err := url.Parse(rawURL)
	if err != nil {
		log.Fatal("invalid URL")
	}

	override := getQueryStringOverride(key, url, int(numVariations))
	if !reflect.DeepEqual(override, expected) {
		t.Errorf("unexpected result: %v", override)
	}
}

// Namespace inclusion tests
//
// Test parameters: name, id, namespace, result
func jsonTestInNamespace(t *testing.T, test []interface{}) {
	id, ok1 := test[1].(string)
	ns, ok2 := test[2].([]interface{})
	expected, ok3 := test[3].(bool)
	if !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	namespace := BuildNamespace(ns)
	result := namespace.inNamespace(id)
	if result != expected {
		t.Errorf("unexpected result: %v", result)
	}
}

// Equal weight calculation tests.
//
// Test parameters: numVariations, result
func jsonTestGetEqualWeights(t *testing.T, test []interface{}) {
	numVariations, ok0 := test[0].(float64)
	exp, ok1 := test[1].([]interface{})
	if !ok0 || !ok1 {
		log.Fatal("unpacking test data")
	}

	expected := make([]float64, len(exp))
	for i, e := range exp {
		expected[i] = e.(float64)
	}

	result := getEqualWeights(int(numVariations))
	if !reflect.DeepEqual(round(result), round(expected)) {
		t.Errorf("unexpected value: %v", result)
	}
}

// Decryption function tests.
//
// Test parameters: name, encryptedString, key, expected
func jsonTestDecrypt(t *testing.T, test []interface{}) {
	encryptedString, ok1 := test[1].(string)
	key, ok2 := test[2].(string)
	if !ok1 || !ok2 {
		log.Fatal("unpacking test data")
	}
	nilExpected := test[3] == nil
	expected := ""
	if !nilExpected {
		expected, ok2 = test[3].(string)
		if !ok2 {
			log.Fatal("unpacking test data")
		}
	}

	result, err := decrypt(encryptedString, key)
	if nilExpected {
		if err == nil {
			t.Errorf("expected error return")
		}
	} else {
		if err != nil {
			t.Errorf("error in decrypt: %v", err)
		} else if !reflect.DeepEqual(result, expected) {
			t.Errorf("unexpected result: %v", result)
			fmt.Printf("expected: '%s' (%d)\n", expected, len(expected))
			fmt.Println([]byte(expected))
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
	fn func(t *testing.T, test []interface{})) {
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
				fn(t, test)
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
	fn func(t *testing.T, label string, test []interface{})) {
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
			test, ok := gtest.([]interface{})
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
				fn(t, name, test)
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
