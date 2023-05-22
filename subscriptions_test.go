package growthbook

import (
	"reflect"
	"testing"
)

func TestSubscriptionsSubscribe(t *testing.T) {
	context := NewContext().WithAttributes(Attributes{"id": "1"})
	gb := New(context)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")

	var savedExp *Experiment
	called := 0
	gb.Subscribe(func(exp *Experiment, result *Result) {
		savedExp = exp
		called++
	})

	gb.Run(exp1)
	gb.Run(exp1)

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

	gb.ClearSavedResults()
	gb.Run(exp1)
	// Change attributes to change experiment result so subscription
	// gets triggered twice.
	gb.WithAttributes(Attributes{"id": "3"})
	gb.Run(exp1)

	if !reflect.DeepEqual(savedExp, exp1) {
		t.Errorf("unexpected experiment value: %v", savedExp)
	}
	if called != 2 {
		t.Errorf("expected called = 2, got called = %d", called)
	}
}

func TestSubscriptionsUnsubscribe(t *testing.T) {
	context := NewContext().WithAttributes(Attributes{"id": "1"})
	gb := New(context)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")

	var savedExp *Experiment
	called := 0
	unsubscribe := gb.Subscribe(func(exp *Experiment, result *Result) {
		savedExp = exp
		called++
	})

	gb.Run(exp1)
	gb.WithAttributes(Attributes{"id": "3"})
	unsubscribe()
	gb.Run(exp1)

	if !reflect.DeepEqual(savedExp, exp1) {
		t.Errorf("unexpected experiment value: %v", savedExp)
	}
	if called != 1 {
		t.Errorf("expected called = 1, got called = %d", called)
	}
}

func TestSubscriptionsTrack(t *testing.T) {
	context := NewContext().WithAttributes(Attributes{"id": "1"})
	gb := New(context)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")
	exp2 := NewExperiment("experiment-2").WithVariations("result3", "result4")

	called := 0
	context.WithTrackingCallback(func(exp *Experiment, result *Result) {
		called++
	})

	gb.Run(exp1)
	gb.Run(exp2)
	gb.Run(exp1)
	gb.Run(exp2)
	gb.WithAttributes(Attributes{"id": "3"})
	gb.Run(exp1)
	gb.Run(exp2)
	gb.Run(exp1)
	gb.Run(exp2)
	if called != 4 {
		t.Errorf("expected called = 4, got called = %d", called)
	}
}

func TestSubscriptionsRetrieve(t *testing.T) {
	context := NewContext().WithAttributes(Attributes{"id": "1"})
	gb := New(context)
	exp1 := NewExperiment("experiment-1").WithVariations("result1", "result2")
	exp2 := NewExperiment("experiment-2").WithVariations("result3", "result4")

	gb.Run(exp1)
	gb.Run(exp2)
	resultsLen := len(gb.GetAllResults())
	if resultsLen != 2 {
		t.Errorf("expected results length = 2, got length = %d", resultsLen)
	}
}
