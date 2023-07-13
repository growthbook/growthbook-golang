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
	tr := track()
	client := NewClient(&Options{TrackingCallback: tr.cb})

	exp1 := NewExperiment("my-tracked-test").WithVariations(0, 1)
	exp2 := NewExperiment("my-other-tracked-test").WithVariations(0, 1)

	res1, err := client.Run(exp1, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	_, err = client.Run(exp1, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	_, err = client.Run(exp1, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	res4, err := client.Run(exp2, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	res5, err := client.Run(exp2, Attributes{"id": "2"})
	if err != nil {
		t.Error("unexpected error:", err)
	}

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
	client := NewClient(nil).
		WithOverrides(ExperimentOverrides{
			"forced-test": &ExperimentOverride{
				Force: &forceVal,
			}})

	res, err := client.Run(NewExperiment("forced-test").WithVariations(0, 1),
		Attributes{"id": "6"})
	if err != nil {
		t.Error("unexpected error:", err)
	}

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
	client := NewClient(nil).
		WithOverrides(ExperimentOverrides{
			"my-test": &ExperimentOverride{
				Coverage: &overrideVal,
			}})

	res, err := client.Run(NewExperiment("my-test").WithVariations(0, 1), Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}

	if res.VariationID != 0 {
		t.Error("expected variation ID 0, got", res.VariationID)
	}
	if res.InExperiment != false {
		t.Error("expected InExperiment to be false")
	}
}

func TestExperimentDoesNotTrackWhenForcedWithOverrides(t *testing.T) {
	tr := track()
	client := NewClient(&Options{TrackingCallback: tr.cb})
	exp := NewExperiment("forced-test").WithVariations(0, 1)

	forceVal := 1
	client = client.WithOverrides(ExperimentOverrides{
		"forced-test": &ExperimentOverride{Force: &forceVal},
	})

	client.Run(exp, Attributes{"id": "6"})

	if len(tr.calls) != 0 {
		t.Error("expected 0 calls to tracking callback, got ", len(tr.calls))
	}
}

func TestExperimentURLFromOverrides(t *testing.T) {
	urlRe := regexp.MustCompile(`^\/path`)
	client := NewClient(nil).
		WithOverrides(ExperimentOverrides{
			"my-test": &ExperimentOverride{URL: urlRe},
		})

	res, err := client.Run(NewExperiment("my-test").WithVariations(0, 1),
		Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res.InExperiment != false {
		t.Error("expected InExperiment to be false")
	}
}

func TestExperimentFiltersUserGroups(t *testing.T) {
	client := NewClient(&Options{
		Groups: map[string]bool{
			"alpha":    true,
			"beta":     true,
			"internal": false,
			"qa":       false,
		},
	})

	exp := NewExperiment("my-test").
		WithVariations(0, 1).
		WithGroups("internal", "qa")
	res, err := client.Run(exp, Attributes{"id": "123"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res.InExperiment != false {
		t.Error("1: expected InExperiment to be false")
	}

	exp = NewExperiment("my-test").
		WithVariations(0, 1).
		WithGroups("internal", "qa", "beta")
	res, err = client.Run(exp, Attributes{"id": "123"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res.InExperiment != true {
		t.Error("2: expected InExperiment to be true")
	}

	exp = NewExperiment("my-test").
		WithVariations(0, 1)
	res, err = client.Run(exp, Attributes{"id": "123"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res.InExperiment != true {
		t.Error("3: expected InExperiment to be true")
	}
}

func TestExperimentCustomIncludeCallback(t *testing.T) {
	client := NewClient(nil)

	exp := NewExperiment("my-test").
		WithVariations(0, 1).
		WithIncludeFunction(func() bool { return false })

	res, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res.InExperiment != false {
		t.Error("expected InExperiment to be false")
	}
}

func TestExperimentQuerystringForceDisablsTracking(t *testing.T) {
	tr := track()
	client := NewClient(&Options{
		TrackingCallback: tr.cb,
		URL:              mustParseUrl("http://example.com?forced-test-qs=1"),
	})

	_, err := client.Run(NewExperiment("forced-test-qs").WithVariations(0, 1),
		Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}

	if len(tr.calls) != 0 {
		t.Errorf("expected 0 calls to tracking callback, got %d", len(tr.calls))
	}
}

func TestExperimentURLTargeting(t *testing.T) {
	exp := NewExperiment("my-test").
		WithVariations(0, 1).
		WithURL(regexp.MustCompile("^/post/[0-9]+"))

	check := func(icase int, url string, inExperiment bool, value interface{}) {
		client := NewClient(&Options{URL: mustParseUrl(url)})

		result, err := client.Run(exp, Attributes{"id": "1"})
		if err != nil {
			t.Error("unexpected error:", err)
		}
		if result.InExperiment != inExperiment {
			t.Errorf("%d: expected InExperiment = %v, got %v",
				icase, inExperiment, result.InExperiment)
		}
		if !reflect.DeepEqual(result.Value, value) {
			t.Errorf("%d: expected value = %v, got %v",
				icase, value, result.Value)
		}
	}

	check(1, "http://example.com", false, 0)
	check(2, "http://example.com/post/123", true, 1)
	exp = exp.WithURL(regexp.MustCompile("http://example.com/post/[0-9]+"))
	check(3, "http://example.com/post/123", true, 1)
}

func TestExperimentIgnoresDraftExperiments(t *testing.T) {
	client := NewClient(nil)

	exp := NewExperiment("my-test").
		WithStatus(DraftStatus).
		WithVariations(0, 1)

	res1, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}

	client = NewClient(&Options{
		URL: mustParseUrl("http://example.com/?my-test=1"),
	})
	res2, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}

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
	client := NewClient(nil)

	expLose := NewExperiment("my-test").
		WithStatus(StoppedStatus).
		WithVariations(0, 1, 2)
	expWin := NewExperiment("my-test").
		WithStatus(StoppedStatus).
		WithVariations(0, 1, 2).
		WithForce(2)

	res1, err := client.Run(expLose, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	res2, err := client.Run(expWin, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}

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
	client := NewClient(nil)

	// Full coverage
	exp := NewExperiment("my-test").WithVariations(0, 1)
	variations := map[string]int{
		"0":  0,
		"1":  0,
		"-1": 0,
	}
	countVariations(t, client, exp, 1000, variations)
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
	countVariations(t, client, exp, 10000, variations)
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
	countVariations(t, client, exp, 10000, variations)
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
	client := NewClient(nil)

	exp := NewExperiment("my-test").
		WithVariations(0, 1)

	res1, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	commonCheck(t, 1, res1, true, true, 1)

	client = client.WithForcedVariations(ForcedVariationsMap{
		"my-test": 0,
	})
	res2, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	commonCheck(t, 2, res2, true, false, 0)

	client = client.WithForcedVariations(nil)
	res3, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	commonCheck(t, 3, res3, true, true, 1)
}

func TestExperimentOnceForcesAllVariationsInQAMode(t *testing.T) {
	client := NewClient(&Options{QAMode: true})

	exp := NewExperiment("my-test").
		WithVariations(0, 1)

	res1, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	commonCheck(t, 1, res1, false, false, 0)

	// Still works if explicitly forced
	client = client.WithForcedVariations(ForcedVariationsMap{"my-test": 1})
	res2, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	commonCheck(t, 2, res2, true, false, 1)

	// Works if the experiment itself is forced
	exp2 := NewExperiment("my-test-2").WithVariations(0, 1).WithForce(1)
	res3, err := client.Run(exp2, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	commonCheck(t, 3, res3, true, false, 1)
}

func TestExperimentFiresSubscriptionsCorrectly(t *testing.T) {
	client := NewClient(nil)

	fired := false
	checkFired := func(icase int, f bool) {
		if fired != f {
			t.Errorf("%d: expected fired to be %v", icase, f)
		}
	}

	unsubscriber := client.Subscribe(func(experiment *Experiment, result *Result) {
		fired = true
	})
	checkFired(1, false)

	exp := NewExperiment("my-test").WithVariations(0, 1)

	// Should fire when user is put in an experiment
	_, err := client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	checkFired(2, true)

	// Does not fire if nothing has changed
	fired = false
	_, err = client.Run(exp, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	checkFired(3, false)

	// Does not fire after unsubscribed
	unsubscriber()
	exp2 := NewExperiment("other-test").WithVariations(0, 1)
	_, err = client.Run(exp2, Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	checkFired(4, false)
}

func TestExperimentStoresAssignedVariations(t *testing.T) {
	client := NewClient(nil)
	_, err := client.Run(NewExperiment("my-test").WithVariations(0, 1), Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	_, err = client.Run(NewExperiment("my-test-3").WithVariations(0, 1), Attributes{"id": "1"})
	if err != nil {
		t.Error("unexpected error:", err)
	}

	assignedVars := client.GetAllResults()

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
	client := NewClient(nil)

	variations := map[string]int{
		"0":  0,
		"1":  0,
		"-1": 0,
	}

	exp := NewExperiment("my-test").
		WithVariations(0, 1).
		WithNamespace(&Namespace{"namespace", 0.0, 0.5})
	countVariations(t, client, exp, 10000, variations)

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

func countVariations(t *testing.T, client *Client,
	exp *Experiment, runs int, variations map[string]int) {
	for i := 0; i < runs; i++ {
		res, err := client.Run(exp, Attributes{"id": fmt.Sprint(i)})
		if err != nil {
			t.Error("unexpected error:", err)
		}
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
