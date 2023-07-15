package growthbook

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestFeaturesCanSetFeatures(t *testing.T) {
	client := NewClient(nil).
		WithFeatures(FeatureMap{"feature": &Feature{DefaultValue: 0}})

	result, err := client.EvalFeature("feature", Attributes{"id": "123"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
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
	client := NewClient(nil)

	keyString := "Ns04T5n9+59rl2x3SlNHtQ=="
	encrypedFeatures :=
		"vMSg2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	client, err := client.WithEncryptedFeatures(encrypedFeatures, keyString)
	if err != nil {
		t.Error("unexpected error: ", err)
	}

	expectedJson := `{
    "testfeature1": {
      "defaultValue": true,
      "rules": [{"condition": { "id": "1234" }, "force": false}]
    }
  }`
	expected := FeatureMap{}
	err = json.Unmarshal([]byte(expectedJson), &expected)
	if err != nil {
		t.Errorf("failed to parse expected JSON: %s", expectedJson)
	}

	actual := client.Features()

	if !reflect.DeepEqual(actual, expected) {
		t.Error("unexpected features value: ", actual)
	}
}

func TestFeaturesDecryptFeaturesWithInvalidKey(t *testing.T) {
	client := NewClient(nil)

	keyString := "fakeT5n9+59rl2x3SlNHtQ=="
	encrypedFeatures :=
		"vMSg2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	_, err := client.WithEncryptedFeatures(encrypedFeatures, keyString)
	if err == nil {
		t.Error("unexpected lack of error")
	}
}

func TestFeaturesDecryptFeaturesWithInvalidCiphertext(t *testing.T) {
	client := NewClient(nil)

	keyString := "Ns04T5n9+59rl2x3SlNHtQ=="
	encrypedFeatures :=
		"FAKE2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	_, err := client.WithEncryptedFeatures(encrypedFeatures, keyString)
	if err == nil {
		t.Error("unexpected lack of error")
	}
}

func TestFeaturesReturnsRuleID(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "id": "foo"}]}
  }`
	features := FeatureMap{}
	err := json.Unmarshal([]byte(featuresJson), &features)
	if err != nil {
		t.Errorf("failed to parse expected JSON: %s", featuresJson)
	}
	client := NewClient(nil).
		WithFeatures(features)
	result, err := client.EvalFeature("feature", Attributes{})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if result.RuleID != "foo" {
		t.Errorf("expected rule ID to be foo, got: %v", result.RuleID)
	}
}

func TestFeaturesUsesAttributeOverrides(t *testing.T) {
	attrs := Attributes{"id": "123", "foo": "bar"}
	client := NewClient(nil).
		WithAttributeOverrides(Attributes{"foo": "baz"})

	exp1 := NewExperiment("my-test").WithVariations(0, 1).WithHashAttribute("foo")
	result, err := client.Run(exp1, attrs)
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if result.HashValue != "baz" {
		t.Errorf("unexpected experiment result: %v\n", result.HashValue)
	}

	client = client.WithAttributeOverrides(nil)

	result, err = client.Run(exp1, attrs)
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if result.HashValue != "bar" {
		t.Errorf("unexpected experiment result: %v\n", result.HashValue)
	}
}

func TestFeaturesUsesForcedFeatureValues(t *testing.T) {
	featuresJson := `{
    "feature1": {"defaultValue": 0},
    "feature2": {"defaultValue": 0}
  }`
	features := FeatureMap{}
	err := json.Unmarshal([]byte(featuresJson), &features)
	if err != nil {
		t.Errorf("failed to parse expected JSON: %s", featuresJson)
	}
	client := NewClient(nil).
		WithFeatures(features).
		WithForcedFeatures(map[string]interface{}{
			"feature2": 1.0,
			"feature3": 1.0,
		})

	check := func(icase int, feature string, value interface{}) {
		result, err := client.EvalFeature(feature, nil)
		if err != nil {
			t.Error("unexpected error:", err)
		}
		if !reflect.DeepEqual(result.Value, value) {
			t.Errorf("%d: result from EvalFeature: expected %v, got %v",
				icase, value, result.Value)
		}
	}

	check(1, "feature1", 0.0)
	check(2, "feature2", 1.0)
	check(3, "feature3", 1.0)

	client = client.WithForcedFeatures(nil)

	check(4, "feature1", 0.0)
	check(5, "feature2", 0.0)
	check(6, "feature3", nil)
}

func TestFeaturesGetsFeatures(t *testing.T) {
	featuresJson := `{ "feature1": { "defaultValue": 0 } }`
	features := FeatureMap{}
	err := json.Unmarshal([]byte(featuresJson), &features)
	if err != nil {
		t.Errorf("failed to parse expected JSON: %s", featuresJson)
	}
	client := NewClient(nil).WithFeatures(features)

	if !reflect.DeepEqual(client.Features(), features) {
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
	features := FeatureMap{}
	err := json.Unmarshal([]byte(featuresJson), &features)
	if err != nil {
		t.Errorf("failed to parse expected JSON: %s", featuresJson)
	}

	type featureCall struct {
		key    string
		result *FeatureResult
	}
	calls := []featureCall{}
	callback := func(ctx context.Context, key string, result *FeatureResult) {
		calls = append(calls, featureCall{key, result})
	}
	client := NewClient(&Options{OnFeatureUsage: callback}).
		WithFeatures(features)

	// Fires for regular features
	res1, err := client.EvalFeature("feature", Attributes{"color": "green"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res1.Value != 0.0 {
		t.Errorf("expected value 0, got %#v", res1.Value)
	}

	// Fires when the assigned value changes
	res2, err := client.EvalFeature("feature", Attributes{"color": "blue"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
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
	featuresJson := `{"feature": {"defaultValue": "blue"}}`
	features := FeatureMap{}
	err := json.Unmarshal([]byte(featuresJson), &features)
	if err != nil {
		t.Errorf("failed to parse expected JSON: %s", featuresJson)
	}
	client := NewClient(nil).WithFeatures(features)

	res, err := client.GetFeatureValue("feature", nil, "green")
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res != "blue" {
		t.Error("1: unexpected return from GetFeatureValue: ", res)
	}
	res, err = client.GetFeatureValue("unknown", nil, "green")
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res != "green" {
		t.Error("2: unexpected return from GetFeatureValue: ", res)
	}
	res, err = client.GetFeatureValue("testing", nil, nil)
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if res != nil {
		t.Error("3: unexpected return from GetFeatureValue: ", res)
	}
}

func TestFeaturesUsageTracking(t *testing.T) {
	called := false
	cb := func(ctx context.Context, key string, result *FeatureResult) {
		called = true
	}

	client := NewClient(&Options{OnFeatureUsage: cb}).
		WithFeatures(FeatureMap{"feature": &Feature{DefaultValue: 0}})

	result, err := client.EvalFeature("feature", Attributes{"id": "123"})
	if err != nil {
		t.Error("unexpected error:", err)
	}
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
