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
	d := struct {
		name      string
		condition *Condition
		value     map[string]interface{}
		expected  bool
	}{}
	unmarshalTest(test, []interface{}{&d.name, &d.condition, &d.value, &d.expected})

	attrs := Attributes(d.value)
	result := d.condition.Eval(attrs)
	if !reflect.DeepEqual(result, d.expected) {
		t.Errorf("unexpected result: %v", result)
	}
}

// Version comparison tests.
//
// Test parameters: ...
func jsonTestVersionCompare(t *testing.T, comparison string, test []byte) {
	d := struct {
		v1       string
		v2       string
		expected bool
	}{}
	unmarshalTest(test, []interface{}{&d.v1, &d.v2, &d.expected})

	pv1 := paddedVersionString(d.v1)
	pv2 := paddedVersionString(d.v2)

	switch comparison {
	case "eq":
		if (pv1 == pv2) != d.expected {
			t.Errorf("unexpected result: '%s' eq '%s' => %v", d.v1, d.v2, pv1 == pv2)
		}
	case "gt":
		if (pv1 > pv2) != d.expected {
			t.Errorf("unexpected result: '%s' gt '%s' => %v", d.v1, d.v2, pv1 == pv2)
		}
	case "lt":
		if (pv1 < pv2) != d.expected {
			t.Errorf("unexpected result: '%s' lt '%s' => %v", d.v1, d.v2, pv1 == pv2)
		}
	}
}

// Hash function tests.
//
// Test parameters: value, hash
func jsonTestHash(t *testing.T, test []byte) {
	d := struct {
		seed     string
		value    string
		version  int
		expected *float64
	}{}
	unmarshalTest(test, []interface{}{&d.seed, &d.value, &d.version, &d.expected})

	result := hash(d.seed, d.value, d.version)
	if d.expected == nil {
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	} else {
		if result == nil {
			t.Errorf("expected non-nil result, got nil")
		}
		if !reflect.DeepEqual(*result, *d.expected) {
			t.Errorf("unexpected result: %v", *result)
		}
	}
}

// Bucket range tests.
//
// Test parameters: name, args ([numVariations, coverage, weights]), result
func jsonTestGetBucketRange(t *testing.T, test []byte) {
	d := struct {
		name   string
		args   json.RawMessage
		result [][]float64
	}{}
	unmarshalTest(test, []interface{}{&d.name, &d.args, &d.result})

	args := struct {
		numVariations int
		coverage      float64
		weights       []float64
	}{}
	unmarshalTest(d.args, []interface{}{&args.numVariations, &args.coverage, &args.weights})

	variations := make([]Range, len(d.result))
	for i, v := range d.result {
		variations[i] = Range{v[0], v[1]}
	}

	ranges := roundRanges(getBucketRanges(args.numVariations, args.coverage, args.weights))

	if !reflect.DeepEqual(ranges, variations) {
		t.Errorf("unexpected value: %v", d.result)
	}

	// Handle expected warnings.
	if args.coverage < 0 || args.coverage > 1 {
		if len(testLog.errors) != 0 && len(testLog.warnings) != 1 {
			t.Errorf("expected coverage log warning")
		}
		testLog.reset()
	}
	totalWeights := 0.0
	for _, w := range args.weights {
		totalWeights += w
	}
	if totalWeights != 1 {
		if len(testLog.errors) != 0 && len(testLog.warnings) != 1 {
			t.Errorf("expected weight sum log warning")
		}
		testLog.reset()
	}
	if len(args.weights) != len(d.result) {
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
	d := struct {
		name       string
		context    *testContext
		featureKey string
		expected   FeatureResult
	}{}
	unmarshalTest(test, []interface{}{&d.name, &d.context, &d.featureKey, &d.expected})
	client := NewClient(nil).
		WithFeatures(d.context.Features).
		WithForcedVariations(d.context.ForcedVariations)

	retval := client.EvalFeature(d.featureKey, d.context.Attributes)

	if !reflect.DeepEqual(retval, &d.expected) {
		t.Errorf("unexpected value: %v", retval)
	}

	expectedWarnings := map[string]int{
		"unknown feature key": 1,
		"ignores empty rules": 1,
	}
	handleExpectedWarnings(t, d.name, expectedWarnings)
}

// Experiment tests.
//
// Test parameters: name, context, experiment, value, inExperiment
func jsonTestRun(t *testing.T, test []byte) {
	d := struct {
		name         string
		context      *testContext
		experiment   *Experiment
		result       interface{}
		inExperiment bool
		hashUsed     bool
	}{}
	unmarshalTest(test, []interface{}{&d.name, &d.context, &d.experiment, &d.result, &d.inExperiment, &d.hashUsed})

	opt := Options{Disabled: !d.context.Enabled, QAMode: d.context.QAMode}
	if d.context.URL != "" {
		url, err := url.Parse(d.context.URL)
		if err != nil {
			t.Errorf("invalid URL")
		}
		opt.URL = url
	}
	client := NewClient(&opt).
		WithFeatures(d.context.Features).
		WithForcedVariations(d.context.ForcedVariations)
	result := client.Run(d.experiment, d.context.Attributes)

	if !reflect.DeepEqual(result.Value, d.result) {
		t.Errorf("unexpected result value: %v (should be %v)", result.Value, d.result)
	}
	if result.InExperiment != d.inExperiment {
		t.Errorf("unexpected inExperiment value: %v", result.InExperiment)
	}
	if result.HashUsed != d.hashUsed {
		t.Errorf("unexpected hashUsed value: %v", result.HashUsed)
	}

	expectedWarnings := map[string]int{
		"single variation": 1,
	}
	handleExpectedWarnings(t, d.name, expectedWarnings)
}

// Variation choice tests.
//
// Test parameters: name, hash, ranges, result
func jsonTestChooseVariation(t *testing.T, test []byte) {
	d := struct {
		name   string
		hash   float64
		ranges [][]float64
		result int
	}{}
	unmarshalTest(test, []interface{}{&d.name, &d.hash, &d.ranges, &d.result})

	variations := make([]Range, len(d.ranges))
	for i, v := range d.ranges {
		variations[i] = Range{v[0], v[1]}
	}

	variation := chooseVariation(d.hash, variations)
	if variation != int(d.result) {
		t.Errorf("unexpected result: %d", variation)
	}
}

// Query string override tests
//
// Test parameters: name, experiment key, url, numVariations, result
func jsonTestQueryStringOverride(t *testing.T, test []byte) {
	d := struct {
		name          string
		key           string
		rawURL        string
		numVariations int
		expected      *int
	}{}
	unmarshalTest(test, []interface{}{&d.name, &d.key, &d.rawURL, &d.numVariations, &d.expected})
	url, err := url.Parse(d.rawURL)
	if err != nil {
		log.Fatal("invalid URL")
	}

	override := getQueryStringOverride(d.key, url, d.numVariations)
	if !reflect.DeepEqual(override, d.expected) {
		t.Errorf("unexpected result: %v", override)
	}
}

// Namespace inclusion tests
//
// Test parameters: name, id, namespace, result

func jsonTestInNamespace(t *testing.T, test []byte) {
	d := struct {
		name      string
		id        string
		namespace *Namespace
		expected  bool
	}{}
	unmarshalTest(test, []interface{}{&d.name, &d.id, &d.namespace, &d.expected})

	result := d.namespace.inNamespace(d.id)
	if result != d.expected {
		t.Errorf("unexpected result: %v", result)
	}
}

// Equal weight calculation tests.
//
// Test parameters: numVariations, result
func jsonTestGetEqualWeights(t *testing.T, test []byte) {
	d := struct {
		numVariations int
		expected      []float64
	}{}
	unmarshalTest(test, []interface{}{&d.numVariations, &d.expected})

	result := getEqualWeights(d.numVariations)
	if !reflect.DeepEqual(round(result), round(d.expected)) {
		t.Errorf("unexpected value: %v", result)
	}
}

// Decryption function tests.
//
// Test parameters: name, encryptedString, key, expected
func jsonTestDecrypt(t *testing.T, test []byte) {
	d := struct {
		name      string
		encrypted string
		key       string
		expected  *string
	}{}
	unmarshalTest(test, []interface{}{&d.name, &d.encrypted, &d.key, &d.expected})

	result, err := decrypt(d.encrypted, d.key)
	if d.expected == nil {
		if err == nil {
			t.Errorf("expected error return")
		}
	} else {
		if err != nil {
			t.Errorf("error in decrypt: %v", err)
		} else if !reflect.DeepEqual(result, *d.expected) {
			t.Errorf("unexpected result: %v", result)
			fmt.Printf("expected: '%s' (%d)\n", *d.expected, len(*d.expected))
			fmt.Println([]byte(*d.expected))
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
