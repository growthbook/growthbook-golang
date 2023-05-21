package growthbook

import (
	"reflect"
	"testing"
)

// renders when features are set
// decrypts features with custom SubtleCrypto implementation
// decrypts features using the native SubtleCrypto implementation
// throws when decrypting features with invalid key
// throws when decrypting features with invalid encrypted value
// throws when decrypting features and no SubtleCrypto implementation exists
// returns ruleId when evaluating a feature
// updates attributes with setAttributes
// uses attribute overrides
// uses forced feature values
// gets features
// re-fires feature usage when assigned value changes
// fires real-time usage call
// uses fallbacks get getFeatureValue
// clears realtime timer on destroy
// fires remote tracking calls

// from typed-features.test.ts:
//
// typed features
//   getFeatureValue
//     implements type-safe feature getting
//     implements feature getting without types
//   evalFeature
//     evaluates a feature without using types
//     evaluates a typed feature
//   feature (alias for evalFeature(key))
//     evaluates a feature without using types
//     evaluates a typed feature

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

	t.Run("feature usage tracking", func(t *testing.T) {
		called := false
		cb := func(key string, result *FeatureResult) {
			called = true
		}

		context := NewContext().
			WithAttributes(Attributes{
				"id": "123",
			}).
			WithFeatureUsageCallback(cb)
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
		if !called {
			t.Errorf("expected feature tracking callback to be called")
		}
	})
}
