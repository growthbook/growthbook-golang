package growthbook

import (
	"encoding/json"
	"testing"
)

func TestConditionValueNullOrNotPresent(t *testing.T) {
	condition := Condition{}
	json.Unmarshal([]byte(`{"userId": null}`), &condition)
	result := condition.Eval(Attributes{"userId": nil})
	if result != true {
		t.Error("1: expected condition evaluation to be true")
	}

	json.Unmarshal([]byte(`{}`), &condition)
	result = condition.Eval(Attributes{"userId": nil})
	if result != true {
		t.Error("2: expected condition evaluation to be true")
	}
}

func TestConditionValueIsPresent(t *testing.T) {
	condition := Condition{}
	json.Unmarshal([]byte(`{"userId": null}`), &condition)
	result := condition.Eval(Attributes{"userId": "123"})
	if result != false {
		t.Error("1: expected condition evaluation to be false")
	}
}

func TestConditionValueIsPresentButFalsy(t *testing.T) {
	condition := Condition{}
	json.Unmarshal([]byte(`{"userId": null}`), &condition)
	result := condition.Eval(Attributes{"userId": 0})
	if result != false {
		t.Error("1: expected condition evaluation to be false")
	}
	result = condition.Eval(Attributes{"userId": ""})
	if result != false {
		t.Error("2: expected condition evaluation to be false")
	}
}
