package growthbook

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"
)

type trackCall struct {
	experiment *Experiment
	result     *Result
}

type tracker struct {
	calls []trackCall
	cb    func(experiment *Experiment, result *Result)
}

func track() *tracker {
	t := tracker{[]trackCall{}, nil}
	t.cb = func(experiment *Experiment, result *Result) {
		t.calls = append(t.calls, trackCall{experiment, result})
	}
	return &t
}

func TestExperimentTracking(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"})

	tr := track()
	gb := New(context).WithTrackingCallback(tr.cb)

	exp1 := NewExperiment("my-tracked-test").WithVariations(0, 1)
	exp2 := NewExperiment("my-other-tracked-test").WithVariations(0, 1)

	res1 := gb.Run(exp1)
	gb.Run(exp1)
	gb.Run(exp1)
	res4 := gb.Run(exp2)
	context = context.WithUserAttributes(Attributes{"id": "2"})
	res5 := gb.Run(exp2)

	if len(tr.calls) != 3 {
		t.Errorf("expected 3 calls to tracking callback, got %d", len(tr.calls))
	} else {
		if !reflect.DeepEqual(tr.calls[0], trackCall{exp1, res1}) {
			t.Errorf("unexpected callback result")
		}
		if !reflect.DeepEqual(tr.calls[1], trackCall{exp2, res4}) {
			t.Errorf("unexpected callback result")
		}
		if !reflect.DeepEqual(tr.calls[2], trackCall{exp2, res5}) {
			t.Errorf("unexpected callback result")
		}
	}
}

func TestExperimentForcesVariationFromOverrides(t *testing.T) {
	forceVal := 1
	context := NewContext().
		WithOverrides(ExperimentOverrides{
			"forced-test": &ExperimentOverride{
				Force: &forceVal,
			}})
	gb := New(context).
		WithAttributes(Attributes{"id": "6"})

	res := gb.Run(NewExperiment("forced-test").WithVariations(0, 1))

	if res.VariationID != 1 {
		t.Error("expected variation ID 1, got", res.VariationID)
	}
	if res.InExperiment != true {
		t.Error("expected InExperiment to be true")
	}
	if res.HashUsed != false {
		t.Error("expected HashUsed to be false")
	}
}

func TestExperimentCoverageFromOverrides(t *testing.T) {
	overrideVal := 0.01
	context := NewContext().
		WithOverrides(ExperimentOverrides{
			"my-test": &ExperimentOverride{
				Coverage: &overrideVal,
			}})
	gb := New(context).
		WithAttributes(Attributes{"id": "1"})

	res := gb.Run(NewExperiment("my-test").WithVariations(0, 1))

	if res.VariationID != 0 {
		t.Error("expected variation ID 0, got", res.VariationID)
	}
	if res.InExperiment != false {
		t.Error("expected InExperiment to be false")
	}
}

func TestExperimentDoesNotTrackWhenForcedWithOverrides(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "6"})
	tr := track()
	gb := New(context).WithTrackingCallback(tr.cb)
	exp := NewExperiment("forced-test").WithVariations(0, 1)

	forceVal := 1
	context = context.WithOverrides(ExperimentOverrides{
		"forced-test": &ExperimentOverride{Force: &forceVal},
	})

	gb.Run(exp)

	if len(tr.calls) != 0 {
		t.Error("expected 0 calls to tracking callback, got ", len(tr.calls))
	}
}

func TestExperimentURLFromOverrides(t *testing.T) {
	urlRe := regexp.MustCompile(`^\/path`)
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"}).
		WithOverrides(ExperimentOverrides{
			"my-test": &ExperimentOverride{URL: urlRe},
		})
	gb := New(context)

	if gb.Run(NewExperiment("my-test").WithVariations(0, 1)).InExperiment != false {
		t.Error("expected InExperiment to be false")
	}
}

func TestExperimentFiltersUserGroups(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "123"}).
		WithGroups(map[string]bool{
			"alpha":    true,
			"beta":     true,
			"internal": false,
			"qa":       false,
		})
	gb := New(context)

	exp := NewExperiment("my-test").
		WithVariations(0, 1).
		WithGroups("internal", "qa")
	if gb.Run(exp).InExperiment != false {
		t.Error("1: expected InExperiment to be false")
	}

	exp = NewExperiment("my-test").
		WithVariations(0, 1).
		WithGroups("internal", "qa", "beta")
	if gb.Run(exp).InExperiment != true {
		t.Error("2: expected InExperiment to be true")
	}

	exp = NewExperiment("my-test").
		WithVariations(0, 1)
	if gb.Run(exp).InExperiment != true {
		t.Error("3: expected InExperiment to be true")
	}
}

func TestExperimentSetsAttributes(t *testing.T) {
	attributes := Attributes{
		"id":      "1",
		"browser": "firefox",
	}
	gb := New(nil).WithAttributes(attributes)

	if !reflect.DeepEqual(gb.Attributes(), attributes) {
		t.Error("expected attributes to match")
	}
}

func TestExperimentCustomIncludeCallback(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"})
	gb := New(context)

	exp := NewExperiment("my-test").
		WithVariations(0, 1).
		WithIncludeFunction(func() bool { return false })

	if gb.Run(exp).InExperiment != false {
		t.Error("expected InExperiment to be false")
	}
}

func TestExperimentTrackingSkippedWhenContextDisabled(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"}).
		WithEnabled(false)
	tr := track()
	gb := New(context).WithTrackingCallback(tr.cb)

	gb.Run(NewExperiment("disabled-test").WithVariations(0, 1))

	if len(tr.calls) != 0 {
		t.Errorf("expected 0 calls to tracking callback, got %d", len(tr.calls))
	}
}

func TestExperimentQuerystringForceDisablsTracking(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"}).
		WithURL(mustParseUrl("http://example.com?forced-test-qs=1"))
	tr := track()
	gb := New(context).WithTrackingCallback(tr.cb)

	gb.Run(NewExperiment("forced-test-qs").WithVariations(0, 1))

	if len(tr.calls) != 0 {
		t.Errorf("expected 0 calls to tracking callback, got %d", len(tr.calls))
	}
}

func TestExperimentURLTargeting(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"}).
		WithURL(mustParseUrl("http://example.com"))
	gb := New(context)

	exp := NewExperiment("my-test").
		WithVariations(0, 1).
		WithURL(regexp.MustCompile("^/post/[0-9]+"))

	check := func(icase int, e *Experiment, inExperiment bool, value interface{}) {
		result := gb.Run(e)
		if result.InExperiment != inExperiment {
			t.Errorf("%d: expected InExperiment = %v, got %v",
				icase, inExperiment, result.InExperiment)
		}
		if !reflect.DeepEqual(result.Value, value) {
			t.Errorf("%d: expected value = %v, got %v",
				icase, value, result.Value)
		}
	}

	check(1, exp, false, 0)

	context = context.WithURL(mustParseUrl("http://example.com/post/123"))
	check(2, exp, true, 1)

	exp.URL = regexp.MustCompile("http://example.com/post/[0-9]+")
	check(3, exp, true, 1)
}

func TestExperimentIgnoresDraftExperiments(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"})
	gb := New(context)

	exp := NewExperiment("my-test").
		WithStatus(DraftStatus).
		WithVariations(0, 1)

	res1 := gb.Run(exp)
	context = context.WithURL(mustParseUrl("http://example.com/?my-test=1"))
	res2 := gb.Run(exp)

	if res1.InExperiment != false {
		t.Error("1: expected InExperiment to be false")
	}
	if res1.HashUsed != false {
		t.Error("1: expected HashUsed to be false")
	}
	if res1.Value != 0 {
		t.Errorf("1: expected Value to be 0, got %v", res1.Value)
	}

	if res2.InExperiment != true {
		t.Error("2: expected InExperiment to be true")
	}
	if res2.HashUsed != false {
		t.Error("2: expected HashUsed to be false")
	}
	if res2.Value != 1 {
		t.Errorf("2: expected Value to be 1, got %v", res2.Value)
	}
}

func TestExperimentIgnoresStoppedExperimentsUnlessForced(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"})
	gb := New(context)

	expLose := NewExperiment("my-test").
		WithStatus(StoppedStatus).
		WithVariations(0, 1, 2)
	expWin := NewExperiment("my-test").
		WithStatus(StoppedStatus).
		WithVariations(0, 1, 2).
		WithForce(2)

	res1 := gb.Run(expLose)
	res2 := gb.Run(expWin)

	if res1.InExperiment != false {
		t.Error("1: expected InExperiment to be false")
	}
	if res1.HashUsed != false {
		t.Error("1: expected HashUsed to be false")
	}
	if res1.Value != 0 {
		t.Errorf("1: expected Value to be 0, got %v", res1.Value)
	}

	if res2.InExperiment != true {
		t.Error("2: expected InExperiment to be true")
	}
	if res2.HashUsed != false {
		t.Error("2: expected HashUsed to be false")
	}
	if res2.Value != 2 {
		t.Errorf("2: expected Value to be 2, got %v", res2.Value)
	}
}

func TestExperimentDoesEvenWeighting(t *testing.T) {
	context := NewContext()
	gb := New(context)

	// Full coverage
	exp := NewExperiment("my-test").WithVariations(0, 1)
	variations := map[string]int{
		"0":  0,
		"1":  0,
		"-1": 0,
	}
	countVariations(t, context, gb, exp, 1000, variations)
	if variations["0"] != 503 {
		t.Errorf("1: expected variations[\"0\"] to be 503, got %v", variations["0"])
	}

	// Reduced coverage
	exp = exp.WithCoverage(0.4)
	variations = map[string]int{
		"0":  0,
		"1":  0,
		"-1": 0,
	}
	countVariations(t, context, gb, exp, 10000, variations)
	if variations["0"] != 2044 {
		t.Errorf("2: expected variations[\"0\"] to be 2044, got %v", variations["0"])
	}
	if variations["1"] != 1980 {
		t.Errorf("2: expected variations[\"1\"] to be 1980, got %v", variations["0"])
	}
	if variations["-1"] != 5976 {
		t.Errorf("2: expected variations[\"0\"] to be 5976, got %v", variations["0"])
	}

	// 3-way
	exp = exp.WithCoverage(0.6).WithVariations(0, 1, 2)
	variations = map[string]int{
		"0":  0,
		"1":  0,
		"2":  0,
		"-1": 0,
	}
	countVariations(t, context, gb, exp, 10000, variations)
	expected := map[string]int{
		"-1": 3913,
		"0":  2044,
		"1":  2000,
		"2":  2043,
	}
	if !reflect.DeepEqual(variations, expected) {
		t.Errorf("3: expected variations counts %#v, git %#v", expected, variations)
	}
}

func TestExperimentForcesMultipleVariationsAtOnce(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"})
	gb := New(context)

	exp := NewExperiment("my-test").
		WithVariations(0, 1)

	res1 := gb.Run(exp)
	commonCheck(t, 1, res1, true, true, 1)

	gb = gb.WithForcedVariations(ForcedVariationsMap{
		"my-test": 0,
	})
	res2 := gb.Run(exp)
	commonCheck(t, 2, res2, true, false, 0)

	gb = gb.WithForcedVariations(nil)
	res3 := gb.Run(exp)
	commonCheck(t, 3, res3, true, true, 1)
}

func TestExperimentOnceForcesAllVariationsInQAMode(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"}).
		WithQAMode(true)
	gb := New(context)

	exp := NewExperiment("my-test").
		WithVariations(0, 1)

	res1 := gb.Run(exp)
	commonCheck(t, 1, res1, false, false, 0)

	// Still works if explicitly forced
	context = context.WithForcedVariations(ForcedVariationsMap{"my-test": 1})
	res2 := gb.Run(exp)
	commonCheck(t, 2, res2, true, false, 1)

	// Works if the experiment itself is forced
	exp2 := NewExperiment("my-test-2").WithVariations(0, 1).WithForce(1)
	res3 := gb.Run(exp2)
	commonCheck(t, 3, res3, true, false, 1)
}

func TestExperimentFiresSubscriptionsCorrectly(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"})
	gb := New(context)

	fired := false
	checkFired := func(icase int, f bool) {
		if fired != f {
			t.Errorf("%d: expected fired to be %v", icase, f)
		}
	}

	unsubscriber := gb.Subscribe(func(experiment *Experiment, result *Result) {
		fired = true
	})
	checkFired(1, false)

	exp := NewExperiment("my-test").WithVariations(0, 1)

	// Should fire when user is put in an experiment
	gb.Run(exp)
	checkFired(2, true)

	// Does not fire if nothing has changed
	fired = false
	gb.Run(exp)
	checkFired(3, false)

	// Does not fire after unsubscribed
	unsubscriber()
	exp2 := NewExperiment("other-test").WithVariations(0, 1)
	gb.Run(exp2)
	checkFired(4, false)
}

func TestExperimentStoresAssignedVariations(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"})
	gb := New(context)
	gb.Run(NewExperiment("my-test").WithVariations(0, 1))
	gb.Run(NewExperiment("my-test-3").WithVariations(0, 1))

	assignedVars := gb.GetAllResults()

	if len(assignedVars) != 2 {
		t.Errorf("expected len(assignedVars) to be 2, got %d", len(assignedVars))
	}
	if assignedVars["my-test"].Result.VariationID != 1 {
		t.Errorf("expected assignedVars[\"my-test\"] to be 1, got %d",
			assignedVars["my-test"].Result.VariationID)
	}
	if assignedVars["my-test-3"].Result.VariationID != 0 {
		t.Errorf("expected assignedVars[\"my-test-3\"] to be 0, got %d",
			assignedVars["my-test-3"].Result.VariationID)
	}
}

func TestExperimentDoesNotHaveBiasWhenUsingNamespaces(t *testing.T) {
	context := NewContext().
		WithUserAttributes(Attributes{"id": "1"})
	gb := New(context)

	variations := map[string]int{
		"0":  0,
		"1":  0,
		"-1": 0,
	}

	exp := NewExperiment("my-test").
		WithVariations(0, 1).
		WithNamespace(&Namespace{"namespace", 0.0, 0.5})
	countVariations(t, context, gb, exp, 10000, variations)

	expected := map[string]int{
		"-1": 4973,
		"0":  2538,
		"1":  2489,
	}
	if !reflect.DeepEqual(variations, expected) {
		t.Errorf("expected variations counts %#v, git %#v", expected, variations)
	}
}

func commonCheck(t *testing.T, icase int, res *Result,
	inExperiment bool, hashUsed bool, value FeatureValue) {
	if res.InExperiment != inExperiment {
		t.Errorf("%d: expected InExperiment to be %v", icase, inExperiment)
	}
	if res.HashUsed != hashUsed {
		t.Errorf("%d: expected HashUsed to be %v", icase, hashUsed)
	}
	if res.Value != value {
		t.Errorf("3: expected Value to be %#v, got %#v", value, res.Value)
	}
}

func countVariations(t *testing.T, context *Context, gb *GrowthBook,
	exp *Experiment, runs int, variations map[string]int) {
	for i := 0; i < runs; i++ {
		context = context.WithUserAttributes(Attributes{"id": fmt.Sprint(i)})
		res := gb.Run(exp)
		v := -1
		ok := false
		if res.InExperiment {
			v, ok = res.Value.(int)
			if !ok {
				t.Error("non int feature result")
			}
		}
		variations[fmt.Sprint(v)]++
	}
}
