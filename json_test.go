package growthbook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/url"
	"testing"

	. "github.com/franela/goblin"
)

// Main test function for running JSON-based tests. These all use a
// jsonTest helper function to read and parse the JSON test case file.

func TestJSON(t *testing.T) {
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
func jsonTestFeature(g *G, itest int, test []interface{}) {
	name, ok0 := test[0].(string)
	contextDict, ok1 := test[1].(map[string]interface{})
	featureKey, ok2 := test[2].(string)
	expectedDict, ok3 := test[3].(map[string]interface{})
	if !ok0 || !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	g.It(fmt.Sprintf("GrowthBook.Feature[%d] %s", itest, name), func() {
		context := BuildContext(contextDict)
		growthbook := New(context)
		expected := BuildFeatureResult(expectedDict)
		g.Assert(growthbook.Feature(featureKey)).Equal(expected)
	})
}

// Condition evaluation tests.
//
// Test parameters: name, condition, attributes, result
func jsonTestEvalCondition(g *G, itest int, test []interface{}) {
	name, ok0 := test[0].(string)
	condition, ok1 := test[1].(map[string]interface{})
	value, ok2 := test[2].(map[string]interface{})
	expected, ok3 := test[3].(bool)
	if !ok0 || !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	g.It(fmt.Sprintf("Condition.Eval[%d] %s", itest, name), func() {
		cond, err := BuildCondition(condition)
		if err != nil {
			log.Fatal(err)
		}
		attrs := Attributes(value)
		g.Assert(cond.Eval(attrs)).Equal(expected)
	})
}

// Hash function tests.
//
// Test parameters: value, hash
func jsonTestHash(g *G, itest int, test []interface{}) {
	string, ok0 := test[0].(string)
	value, ok1 := test[1].(float64)
	if !ok0 || !ok1 {
		log.Fatal("unpacking test data")
	}

	g.It(fmt.Sprintf("hashFnv32a[%d] %s", itest, string), func() {
		g.Assert(float64(hashFnv32a(string)%1000) / 1000).Equal(value)
	})
}

// Bucket range tests.
//
// Test parameters: name, args ([numVariations, coverage, weights]), result
func jsonTestGetBucketRange(g *G, itest int, test []interface{}) {
	name, ok0 := test[0].(string)
	args, ok1 := test[1].([]interface{})
	result, ok2 := test[2].([]interface{})
	if !ok0 || !ok1 || !ok2 {
		log.Fatal("unpacking test data")
	}

	numVariations, argok0 := args[0].(float64)
	coverage, argok1 := args[1].(float64)
	if !argok0 || !argok1 {
		log.Fatal("unpacking test data")
	}
	var weights []float64
	if args[2] != nil {
		wgts, ok := args[2].([]interface{})
		if !ok {
			log.Fatal("unpacking test data")
		}
		weights = make([]float64, len(wgts))
		for i, w := range wgts {
			weights[i] = w.(float64)
		}
	}

	variations := make([]VariationRange, len(result))
	for i, v := range result {
		vr, ok := v.([]interface{})
		if !ok || len(vr) != 2 {
			log.Fatal("unpacking test data")
		}
		variations[i] = VariationRange{vr[0].(float64), vr[1].(float64)}
	}

	g.It(fmt.Sprintf("getBucketRange[%d] %s", itest, name), func() {
		g.Assert(roundRanges(getBucketRanges(int(numVariations), coverage, weights))).
			Equal(variations)
	})
}

func jsonTestChooseVariation(g *G, itest int, test []interface{}) {
	name, ok0 := test[0].(string)
	hash, ok1 := test[1].(float64)
	ranges, ok2 := test[2].([]interface{})
	result, ok3 := test[3].(float64)
	if !ok0 || !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	variations := make([]VariationRange, len(ranges))
	for i, v := range ranges {
		vr, ok := v.([]interface{})
		if !ok || len(vr) != 2 {
			log.Fatal("unpacking test data")
		}
		variations[i] = VariationRange{vr[0].(float64), vr[1].(float64)}
	}

	g.It(fmt.Sprintf("chooseVariation[%d] %s", itest, name), func() {
		g.Assert(chooseVariation(hash, variations)).Equal(int(result))
	})
}

func jsonTestQueryStringOverride(g *G, itest int, test []interface{}) {
	name, ok0 := test[0].(string)
	key, ok1 := test[1].(string)
	rawURL, ok2 := test[2].(string)
	numVariations, ok3 := test[3].(float64)
	result := test[4]
	var expected *int
	if result != nil {
		tmp := int(result.(float64))
		expected = &tmp
	}
	if !ok0 || !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}
	url, err := url.Parse(rawURL)
	if err != nil {
		log.Fatal("invalid URL")
	}

	g.It(fmt.Sprintf("getQueryStringOverride[%d] %s", itest, name), func() {
		g.Assert(getQueryStringOverride(key, url, int(numVariations))).Equal(expected)
	})
}

func jsonTestInNamespace(g *G, itest int, test []interface{}) {
	name, ok0 := test[0].(string)
	id, ok1 := test[1].(string)
	ns, ok2 := test[2].([]interface{})
	expected, ok3 := test[3].(bool)
	if !ok0 || !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	namespace := BuildNamespace(ns)
	g.It(fmt.Sprintf("inNamespace[%d] %s", itest, name), func() {
		g.Assert(inNamespace(id, namespace)).Equal(expected)
	})
}

func jsonTestGetEqualWeights(g *G, itest int, test []interface{}) {
	numVariations, ok0 := test[0].(float64)
	exp, ok1 := test[1].([]interface{})
	if !ok0 || !ok1 {
		log.Fatal("unpacking test data")
	}

	expected := make([]float64, len(exp))
	for i, e := range exp {
		expected[i] = e.(float64)
	}

	g.It(fmt.Sprintf("getEqualWeights[%d] %v", itest, numVariations), func() {
		g.Assert(round(getEqualWeights(int(numVariations)))).Equal(round(expected))
	})
}

func jsonTestRun(g *G, itest int, test []interface{}) {
	name, ok0 := test[0].(string)
	contextDict, ok1 := test[1].(map[string]interface{})
	experimentDict, ok2 := test[2].(map[string]interface{})
	resultValue := test[3]
	resultInExperiment, ok3 := test[4].(bool)
	if !ok0 || !ok1 || !ok2 || !ok3 {
		log.Fatal("unpacking test data")
	}

	g.It(fmt.Sprintf("GrowthBook.Run[%d] %s", itest, name), func() {
		context := BuildContext(contextDict)
		growthbook := New(context)
		experiment := BuildExperiment(experimentDict)
		result := growthbook.Run(experiment)
		g.Assert(result.Value).Equal(resultValue)
		g.Assert(result.InExperiment).Equal(resultInExperiment)
	})
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
	fn func(g *G, itest int, test []interface{})) {
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
	g := Goblin(t)
	g.Describe("json test suite: "+label, func() {
		for itest, gtest := range cases {
			test, ok := gtest.([]interface{})
			if !ok {
				log.Fatal("unpacking JSON test data")
			}
			fn(g, itest, test)
		}
	})
}

// Helper to round variation ranges for comparison with fixed test
// values.
func roundRanges(ranges []VariationRange) []VariationRange {
	result := make([]VariationRange, len(ranges))
	for i, r := range ranges {
		rmin := math.Round(r.Min*1000000) / 1000000
		rmax := math.Round(r.Max*1000000) / 1000000
		result[i] = VariationRange{rmin, rmax}
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
