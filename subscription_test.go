package growthbook

import (
	"context"
	"reflect"
	"testing"
)

func TestSubscriptionsSubscribe(t *testing.T) {
	client := NewClient(nil)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")

	var savedExp *Experiment
	called := 0
	client.Subscribe(&ExperimentCallback{func(ctx context.Context, exp *Experiment, result *Result) {
		savedExp = exp
		called++
	}})

	client.Run(exp1, Attributes{"id": "1"})
	client.Run(exp1, Attributes{"id": "1"})

	if !reflect.DeepEqual(savedExp, exp1) {
		t.Errorf("unexpected experiment value: %v", savedExp)
	}
	// Subscription only gets triggered once for repeated experiment
	// runs.
	if called != 1 {
		t.Errorf("expected called = 1, got called = %d", called)
	}

	savedExp = nil
	called = 0

	client.ClearSavedResults()
	client.Run(exp1, Attributes{"id": "1"})
	// Change attributes to change experiment result so subscription
	// gets triggered twice.
	client.Run(exp1, Attributes{"id": "3"})

	if !reflect.DeepEqual(savedExp, exp1) {
		t.Errorf("unexpected experiment value: %v", savedExp)
	}
	if called != 2 {
		t.Errorf("expected called = 2, got called = %d", called)
	}
}

func TestSubscriptionsUnsubscribe(t *testing.T) {
	client := NewClient(nil)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")

	var savedExp *Experiment
	called := 0
	unsubscribe := client.Subscribe(&ExperimentCallback{func(ctx context.Context, exp *Experiment, result *Result) {
		savedExp = exp
		called++
	}})

	client.Run(exp1, Attributes{"id": "1"})
	unsubscribe()
	client.Run(exp1, Attributes{"id": "3"})

	if !reflect.DeepEqual(savedExp, exp1) {
		t.Errorf("unexpected experiment value: %v", savedExp)
	}
	if called != 1 {
		t.Errorf("expected called = 1, got called = %d", called)
	}
}

func TestSubscriptionsTrack(t *testing.T) {
	called := 0
	options := Options{
		ExperimentTracker: NewSingleProcessExperimentTrackingCache(
			&ExperimentCallback{func(ctx context.Context, exp *Experiment, result *Result) {
				called++
			}}),
	}
	client := NewClient(&options)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")
	exp2 := NewExperiment("experiment-2").WithVariations("result3", "result4")

	client.Run(exp1, Attributes{"id": "1"})
	client.Run(exp2, Attributes{"id": "1"})
	client.Run(exp1, Attributes{"id": "1"})
	client.Run(exp2, Attributes{"id": "1"})
	client.Run(exp1, Attributes{"id": "3"})
	client.Run(exp2, Attributes{"id": "3"})
	client.Run(exp1, Attributes{"id": "3"})
	client.Run(exp2, Attributes{"id": "3"})
	if called != 4 {
		t.Errorf("expected called = 4, got called = %d", called)
	}
}

func TestSubscriptionsRetrieve(t *testing.T) {
	client := NewClient(nil)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")
	exp2 := NewExperiment("experiment-2").WithVariations("result3", "result4")

	client.Run(exp1, Attributes{"id": "1"})
	client.Run(exp2, Attributes{"id": "1"})
	resultsLen := len(client.GetAllResults())
	if resultsLen != 2 {
		t.Errorf("expected results length = 2, got length = %d", resultsLen)
	}
}

type testTracker struct {
	called int
}

func newTestTracker() *testTracker {
	return &testTracker{called: 0}
}

func (t *testTracker) Track(ctx context.Context,
	c *Client, exp *Experiment, result *Result, extraData interface{}) {
	t.called++
}

func TestSubscriptionsStruct(t *testing.T) {
	tr := newTestTracker()
	options := Options{ExperimentTracker: NewSingleProcessExperimentTrackingCache(tr)}
	client := NewClient(&options)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")
	exp2 := NewExperiment("experiment-2").WithVariations("result3", "result4")

	client.Run(exp1, Attributes{"id": "1"})
	client.Run(exp2, Attributes{"id": "1"})
	client.Run(exp1, Attributes{"id": "1"})
	client.Run(exp2, Attributes{"id": "1"})
	client.Run(exp1, Attributes{"id": "3"})
	client.Run(exp2, Attributes{"id": "3"})
	client.Run(exp1, Attributes{"id": "3"})
	client.Run(exp2, Attributes{"id": "3"})
	if tr.called != 4 {
		t.Errorf("expected called = 4, got called = %d", tr.called)
	}
}
