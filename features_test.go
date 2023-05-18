package growthbook

import (
	"reflect"
	"testing"
)

func TestFeatures(t *testing.T) {
	t.Run("can set features", func(t *testing.T) {
		context := NewContext().
			WithAttributes(Attributes{
				"id": "123",
			})
		growthbook := New(context).
			WithFeatures(FeatureMap{
				"feature": &Feature{
					DefaultValue: 0,
				},
			})

		result := growthbook.Feature("feature")
		expected := FeatureResult{
			Value:  0,
			On:     false,
			Off:    true,
			Source: DefaultValueResultSource,
		}

		if result == nil || !reflect.DeepEqual(*result, expected) {
			t.Errorf("unexpected result: %v", result)
		}
	})

	t.Run("updates attributes with setAttributes", func(t *testing.T) {
		context := NewContext().
			WithAttributes(Attributes{
				"foo": 1,
				"bar": 2,
			})
		growthbook := New(context)
		growthbook = growthbook.WithAttributes(Attributes{"foo": 2, "baz": 3})

		result := context.Attributes
		expected := Attributes{
			"foo": 2,
			"baz": 3,
		}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("unexpected result: %v", result)
		}
	})
}
