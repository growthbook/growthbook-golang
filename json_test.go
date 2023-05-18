package growthbook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/url"
	"reflect"
	"testing"
)

// Main test function for running JSON-based tests. These all use a
// jsonTest helper function to read and parse the JSON test case file.

func TestJSON(t *testing.T) {
	SetLogger(&testLog)
	jsonTest(t, "feature", jsonTestFeature)
	jsonTest(t, "evalCondition", jsonTestEvalCondition)
	jsonTest(t, "hash", jsonTestHash)
	jsonTest(t, "getBucketRange", jsonTestGetBucketRange)
	jsonTest(t, "chooseVariation", jsonTestChooseVariation)
	jsonTest(t, "getQueryStringOverride", jsonTestQueryStringOverride)
	jsonTest(t, "inNamespace", jsonTestInNamespace)
	jsonTest(t, "getEqualWeights", jsonTestGetEqualWeights)
	jsonTest(t, "run", jsonTestRun)
}

// Test functions driven from JSON cases. Each of this has a similar
// structure, first extracting test data from the JSON data into typed
// values, then performing the test.

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
	retval := growthbook.Feature(featureKey)

	if !reflect.DeepEqual(retval, expected) {
		t.Errorf("unexpected value: %v", retval)
	}
}

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
	result := growthbook.Run(experiment)

	if !reflect.DeepEqual(result.Value, resultValue) {
		t.Errorf("unexpected result value: %v", result.Value)
	}
	if result.InExperiment != resultInExperiment {
		t.Errorf("unexpected inExperiment value: %v", result.InExperiment)
	}
	// if icase >= 2 {
	// 	os.Exit(1)
	// }
}

//------------------------------------------------------------------------------
//
//  TEST UTILITIES
//

// Run a set of JSON test cases.
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

// Helper to round variation ranges for comparison with fixed test
// values.
func roundRanges(ranges []Range) []Range {
	result := make([]Range, len(ranges))
	for i, r := range ranges {
		rmin := math.Round(r.Min*1000000) / 1000000
		rmax := math.Round(r.Max*1000000) / 1000000
		result[i] = Range{rmin, rmax}
	}
	return result
}

// Helper to round floating point arrays for test comparison.
func round(vals []float64) []float64 {
	result := make([]float64, len(vals))
	for i, v := range vals {
		result[i] = math.Round(v*1000000) / 1000000
	}
	return result
}

// Logger to capture error and log messages.
type testLogger struct {
	errors   []string
	warnings []string
	info     []string
}

var testLog = testLogger{}

func (log *testLogger) allErrors() string {
	s := ""
	for i, e := range log.errors {
		if i != 0 {
			s += ", "
		}
		s += e
	}
	return s
}

func (log *testLogger) allWarnings() string {
	s := ""
	for i, e := range log.warnings {
		if i != 0 {
			s += ", "
		}
		s += e
	}
	return s
}

func (log *testLogger) reset() {
	log.errors = []string{}
	log.warnings = []string{}
	log.info = []string{}
}

func formatArgs(args ...interface{}) string {
	s := ""
	for i, a := range args {
		if i != 0 {
			s += " "
		}
		s += fmt.Sprint(a)
	}
	return s
}

func (log *testLogger) Error(msg string, args ...interface{}) {
	s := msg
	if len(args) > 0 {
		s += ": " + formatArgs(args...)
	}
	log.errors = append(log.errors, s)
}

func (log *testLogger) Errorf(format string, args ...interface{}) {
	log.errors = append(log.errors, fmt.Sprintf(format, args...))
}

func (log *testLogger) Warn(msg string, args ...interface{}) {
	s := msg
	if len(args) > 0 {
		s += ": " + formatArgs(args...)
	}
	log.warnings = append(log.warnings, s)
}

func (log *testLogger) Warnf(format string, args ...interface{}) {
	log.warnings = append(log.warnings, fmt.Sprintf(format, args...))
}

func (log *testLogger) Info(msg string, args ...interface{}) {
	s := msg
	if len(args) > 0 {
		s += ": " + fmt.Sprint(args...)
	}
	log.info = append(log.info, s)
}

func (log *testLogger) Infof(format string, args ...interface{}) {
	log.info = append(log.info, fmt.Sprintf(format, args...))
}
