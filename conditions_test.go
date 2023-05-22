package growthbook

import "testing"

func TestConditionValueNullOrNotPresent(t *testing.T) {
	condition := ParseCondition([]byte(`{"userId": null}`))
	result := condition.Eval(Attributes{"userId": nil})
	if result != true {
		t.Error("1: expected condition evaluation to be true")
	}

	condition = ParseCondition([]byte(`{}`))
	result = condition.Eval(Attributes{"userId": nil})
	if result != true {
		t.Error("2: expected condition evaluation to be true")
	}
}

func TestConditionValueIsPresent(t *testing.T) {
	condition := ParseCondition([]byte(`{"userId": null}`))
	result := condition.Eval(Attributes{"userId": "123"})
	if result != false {
		t.Error("1: expected condition evaluation to be false")
	}
}

func TestConditionValueIsPresentButFalsy(t *testing.T) {
	condition := ParseCondition([]byte(`{"userId": null}`))
	result := condition.Eval(Attributes{"userId": 0})
	if result != false {
		t.Error("1: expected condition evaluation to be false")
	}
	result = condition.Eval(Attributes{"userId": ""})
	if result != false {
		t.Error("2: expected condition evaluation to be false")
	}
}
