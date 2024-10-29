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

func TestCompare(t *testing.T) {
	tests := []struct {
		comp     string
		x        interface{}
		y        interface{}
		expected bool
	}{
		{"$lt", float64(10), float64(20), true},
		{"$lte", float64(10), float64(10), true},
		{"$gt", float64(20), float64(10), true},
		{"$gte", float64(10), float64(10), true},
		{"$lt", "apple", "banana", true},
		{"$lte", "apple", "apple", true},
		{"$gt", "banana", "apple", true},
		{"$gte", "apple", "apple", true},
		{"$lt", int(10), int(20), true},
		{"$lte", int(10), int(10), true},
		{"$gt", int(20), int(10), true},
		{"$gte", int(10), int(10), true},
		{"$lt", int32(10), int32(20), true},
		{"$lt", int64(10), int64(20), true},
		{"$lt", float64(20), float64(10), false},
		{"$lte", float64(20), float64(10), false},
		{"$gt", float64(10), float64(20), false},
		{"$gte", float64(10), float64(20), false},
		{"$lt", int(10), "banana", false},
		{"$lt", "apple", int64(10), false},
		{"$lt", float64(10), "banana", false},
		{"$lt", "banana", "apple", false},
		{"$lte", "banana", "apple", false},
		{"$gt", "apple", "banana", false},
		{"$gte", "apple", "banana", false},
		{"$lt", int(20), int(10), false},
		{"$lte", int(20), int(10), false},
		{"$gt", int(10), int(20), false},
		{"$gte", int(10), int(20), false},
		{"$lt", "apple", float64(10), false},
		{"$lte", "apple", float64(10), false},
		{"$gt", "apple", float64(10), false},
		{"$gte", "apple", float64(10), false},
		{"$lt", "apple", int(10), false},
		{"$lte", "apple", int(10), false},
		{"$gt", "apple", int(10), false},
		{"$gte", "apple", int(10), false},
	}

	for _, test := range tests {
		result := compare(test.comp, test.x, test.y)
		if result != test.expected {
			t.Errorf("compare(%s, %v, %v) = %v, expected %v", test.comp, test.x, test.y, result, test.expected)
		}
	}
}
