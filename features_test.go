package growthbook

import (
	"reflect"
	"testing"
)

func TestContextMalformedJSON(t *testing.T) {
	SetLogger(&testLog)

	contextJSON := []string{
		`{"enabled": 1}`,
		`{"attributes": 1}`,
		`{"url": 1}`,
		`{"features": 1}`,
		`{"forcedVariations": 1}`,
		`{"forcedVariations": {"abc": 1, "def": "bad"}}`,
		`{"qaMode": 1}`,
		`{"devMode": 1}`,
		`{"userAttributes": 1}`,
		`{"groups": 1}`,
		`{"groups": {"abc": true, "def": "bad"}}`,
		`{"apiHost": 1}`,
		`{"clientKey": 1}`,
		`{"decryptionKey": 1}`,
		`{"overrides": 1}`,
		`{"unknownKey": "some data"}`,
	}

	for _, json := range contextJSON {
		testLog.reset()
		ParseContext([]byte(json)) // discarding result...
		if len(testLog.warnings) != 1 {
			t.Errorf("expected warning from Context JSON parser for: %s", json)
		}
	}
}

func TestFeaturesCanSetFeatures(t *testing.T) {
	context := NewContext().
		WithAttributes(Attributes{"id": "123"})
	gb := New(context).
		WithFeatures(FeatureMap{"feature": &Feature{DefaultValue: 0}})

	result := gb.Feature("feature")
	expected := FeatureResult{
		Value:  0,
		On:     false,
		Off:    true,
		Source: DefaultValueResultSource,
	}

	if result == nil || !reflect.DeepEqual(*result, expected) {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestFeaturesCanSetEncryptedFeatures(t *testing.T) {
	gb := New(nil)

	keyString := "Ns04T5n9+59rl2x3SlNHtQ=="
	encrypedFeatures :=
		"vMSg2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	_, err := gb.WithEncryptedFeatures(encrypedFeatures, keyString)
	if err != nil {
		t.Error("unexpected error: ", err)
	}

	expectedJson := `{
    "testfeature1": {
      "defaultValue": true,
      "rules": [{"condition": { "id": "1234" }, "force": false}]
    }
  }`
	expected := ParseFeatureMap([]byte(expectedJson))
	actual := gb.Features()

	if !reflect.DeepEqual(actual, expected) {
		t.Error("unexpected features value: ", actual)
	}
}

func TestFeaturesDecryptFeaturesWithInvalidKey(t *testing.T) {
	gb := New(nil)

	keyString := "fakeT5n9+59rl2x3SlNHtQ=="
	encrypedFeatures :=
		"vMSg2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	_, err := gb.WithEncryptedFeatures(encrypedFeatures, keyString)
	if err == nil {
		t.Error("unexpected lack of error")
	}
}

func TestFeaturesDecryptFeaturesWithInvalidCiphertext(t *testing.T) {
	gb := New(nil)

	keyString := "Ns04T5n9+59rl2x3SlNHtQ=="
	encrypedFeatures :=
		"FAKE2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	_, err := gb.WithEncryptedFeatures(encrypedFeatures, keyString)
	if err == nil {
		t.Error("unexpected lack of error")
	}
}

func TestFeaturesReturnsRuleID(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "id": "foo"}]}
  }`
	gb := New(nil).
		WithFeatures(ParseFeatureMap([]byte(featuresJson)))
	result := gb.EvalFeature("feature")
	if result.RuleID != "foo" {
		t.Errorf("expected rule ID to be foo, got: %v", result.RuleID)
	}
}

func TestFeaturesUpdatesAttributes(t *testing.T) {
	context := NewContext().
		WithAttributes(Attributes{"foo": 1, "bar": 2})
	gb := New(context).
		WithAttributes(Attributes{"foo": 2, "baz": 3})

	result := gb.Attributes()
	expected := Attributes{"foo": 2, "baz": 3}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("unexpected result: %v", result)
	}
	if !reflect.DeepEqual(gb.Attributes(), expected) {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestFeaturesUsesAttributeOverrides(t *testing.T) {
	context := NewContext().
		WithAttributes(Attributes{"id": "123", "foo": "bar"})
	gb := New(context).
		WithAttributeOverrides(Attributes{"foo": "baz"})

	if !reflect.DeepEqual(gb.Attributes(),
		Attributes{"id": "123", "foo": "baz"}) {
		t.Errorf("unexpected value for gb.Attributes(): %v\n",
			gb.Attributes())
	}

	exp1 := NewExperiment("my-test").WithVariations(0, 1).WithHashAttribute("foo")
	result := gb.Run(exp1)
	if result.HashValue != "baz" {
		t.Errorf("unexpected experiment result: %v\n", result.HashValue)
	}

	gb = gb.WithAttributeOverrides(nil)

	if !reflect.DeepEqual(gb.Attributes(),
		Attributes{"id": "123", "foo": "bar"}) {
		t.Errorf("unexpected value for gb.Attributes(): %v\n",
			gb.Attributes())
	}

	result = gb.Run(exp1)
	if result.HashValue != "bar" {
		t.Errorf("unexpected experiment result: %v\n", result.HashValue)
	}
}

func TestFeaturesUsesForcedFeatureValues(t *testing.T) {
	featuresJson := `{
    "feature1": {"defaultValue": 0},
    "feature2": {"defaultValue": 0}
  }`
	gb := New(nil).
		WithFeatures(ParseFeatureMap([]byte(featuresJson))).
		WithForcedFeatures(map[string]interface{}{
			"feature2": 1.0,
			"feature3": 1.0,
		})

	check := func(icase int, feature string, value interface{}) {
		result := gb.EvalFeature(feature)
		if !reflect.DeepEqual(result.Value, value) {
			t.Errorf("%d: result from EvalFeature: expected %v, got %v",
				icase, value, result.Value)
		}
	}

	check(1, "feature1", 0.0)
	check(2, "feature2", 1.0)
	check(3, "feature3", 1.0)

	gb = gb.WithForcedFeatures(nil)

	check(4, "feature1", 0.0)
	check(5, "feature2", 0.0)
	check(6, "feature3", nil)
}

func TestFeaturesGetsFeatures(t *testing.T) {
	featuresJson := `{ "feature1": { "defaultValue": 0 } }`
	features := ParseFeatureMap([]byte(featuresJson))
	gb := New(nil).WithFeatures(features)

	if !reflect.DeepEqual(gb.Features(), features) {
		t.Error("expected features to match")
	}
}

func TestFeaturesFeatureUsageWhenAssignedValueChanges(t *testing.T) {
	featuresJson := `{
    "feature": {
      "defaultValue": 0,
      "rules": [{"condition": {"color": "blue"}, "force": 1}]
    }
  }`
	context := NewContext().
		WithAttributes(Attributes{"color": "green"}).
		WithFeatures(ParseFeatureMap([]byte(featuresJson)))

	type featureCall struct {
		key    string
		result *FeatureResult
	}
	calls := []featureCall{}
	callback := func(key string, result *FeatureResult) {
		calls = append(calls, featureCall{key, result})
	}
	gb := New(context).WithFeatureUsageCallback(callback)

	// Fires for regular features
	res1 := gb.EvalFeature("feature")
	if res1.Value != 0.0 {
		t.Errorf("expected value 0, got %#v", res1.Value)
	}

	// Fires when the assigned value changes
	gb = gb.WithAttributes(Attributes{"color": "blue"})
	res2 := gb.EvalFeature("feature")
	if res2.Value != 1.0 {
		t.Errorf("expected value 1, got %#v", res2.Value)
	}

	if len(calls) != 2 {
		t.Errorf("expected 2 calls to feature usage callback, got %d", len(calls))
	} else {
		if !reflect.DeepEqual(calls[0], featureCall{"feature", res1}) {
			t.Errorf("unexpected callback result")
		}
		if !reflect.DeepEqual(calls[1], featureCall{"feature", res2}) {
			t.Errorf("unexpected callback result")
		}
	}
}

func TestFeaturesUsesFallbacksForGetFeatureValue(t *testing.T) {
	gb := New(nil).WithFeatures(ParseFeatureMap(
		[]byte(`{"feature": {"defaultValue": "blue"}}`)))

	res := gb.GetFeatureValue("feature", "green")
	if res != "blue" {
		t.Error("1: unexpected return from GetFeatureValue: ", res)
	}
	res = gb.GetFeatureValue("unknown", "green")
	if res != "green" {
		t.Error("2: unexpected return from GetFeatureValue: ", res)
	}
	res = gb.GetFeatureValue("testing", nil)
	if res != nil {
		t.Error("3: unexpected return from GetFeatureValue: ", res)
	}
}

func TestFeaturesUsageTracking(t *testing.T) {
	called := false
	cb := func(key string, result *FeatureResult) {
		called = true
	}

	context := NewContext().
		WithAttributes(Attributes{"id": "123"}).
		WithFeatureUsageCallback(cb)
	gb := New(context).
		WithFeatures(FeatureMap{"feature": &Feature{DefaultValue: 0}})

	result := gb.Feature("feature")
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
}
