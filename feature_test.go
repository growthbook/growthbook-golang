package growthbook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	ctx = context.TODO()
)

func TestJsonMarshaling(t *testing.T) {
	featuresJson := []byte(`{
      "testfeature": {
         "defaultValue": true,
         "rules": [{"condition": { "id": "1234" }, "force": false}]
      }
    }`)

	features := FeatureMap{}
	err := json.Unmarshal(featuresJson, &features)
	require.Nil(t, err)
}

func TestFeaturesDecryptFeaturesWithInvalidKey(t *testing.T) {
	keyString := "fakeT5n9+59rl2x3SlNHtQ=="
	encrypedFeatures :=
		"vMSg2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	client, _ := NewClient(ctx, WithClientKey(keyString))
	err := client.SetEncryptedJSONFeatures(encrypedFeatures)
	require.Error(t, err)
}

func TestFeaturesDecryptFeaturesWithInvalidCiphertext(t *testing.T) {
	keyString := "Ns04T5n9+59rl2x3SlNHtQ=="
	encrypedFeatures :=
		"FAKE2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	client, _ := NewClient(ctx, WithClientKey(keyString))
	err := client.SetEncryptedJSONFeatures(encrypedFeatures)
	require.Error(t, err)
}

func TestFeaturesReturnsRuleID(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "id": "foo"}]}
    }`

	client, _ := NewClient(ctx, WithJsonFeatures(featuresJson))
	result := client.EvalFeature(ctx, "feature")
	require.Equal(t, "foo", result.RuleId)
}

func TestGatesFlagRuleEvaluationOnPrerequisiteFlag(t *testing.T) {
	attributes := Attributes{
		"id":         "123",
		"memberType": "basic",
		"country":    "USA",
	}

	featuresJson := `
    {
		"parentFlag": {
			"defaultValue": "silver",
			"rules": [
				{
					"condition": {
						"country": "Canada"
					},
					"force": "red"
				},
				{
					"condition": {
						"country": {
							"$in": [
								"USA",
								"Mexico"
							]
						}
					},
					"force": "green"
				}
			]
		},
		"childFlag": {
			"defaultValue": "default",
			"rules": [
				{
					"parentConditions": [
						{
							"id": "parentFlag",
							"condition": {
								"value": "green"
							},
							"gate": true
						}
					]
				},
				{
					"condition": {
						"memberType": "basic"
					},
					"force": "success"
				}
			]
		},
		"childFlagWithMissingPrereq": {
			"defaultValue": "default",
			"rules": [
				{
					"parentConditions": [
						{
							"id": "missingParentFlag",
							"condition": {
								"value": "green"
							},
							"gate": true
						}
					]
				}
			]
		}
	}`

	client, _ := NewClient(ctx,
		WithAttributes(attributes),
		WithJsonFeatures(featuresJson),
	)

	missingResult := client.EvalFeature(ctx, "childFlagWithMissingPrereq")
	require.Nil(t, missingResult.Value)

	result1 := client.EvalFeature(ctx, "childFlag")
	require.Equal(t, "success", result1.Value)

	c2, _ := client.WithAttributes(Attributes{
		"id":         "123",
		"memberType": "basic",
		"country":    "Canada",
	})

	result2 := c2.EvalFeature(ctx, "childFlag")
	require.Nil(t, result2.Value)
}

func TestGatesFlagRuleEvaluationOnPrerequisiteExperimentFlag(t *testing.T) {
	attributes := Attributes{
		"id":         "1234",
		"memberType": "basic",
		"country":    "USA",
	}

	featuresJson := `
    {
	"parentExperimentFlag": {
		"defaultValue": 0,
		"rules": [
			{
				"key": "experiment",
				"variations": [
					0,
					1
				],
				"hashAttribute": "id",
				"hashVersion": 2,
				"ranges": [
					[
						0,
						0.5
					],
					[
						0.5,
						1
					]
				]
			}
		]
	},
	"childFlag": {
		"defaultValue": "default",
		"rules": [
			{
				"parentConditions": [
					{
						"id": "parentExperimentFlag",
						"condition": {
							"value": 1
						},
						"gate": true
					}
				]
			},
			{
				"condition": {
					"memberType": "basic"
				},
				"force": "success"
			}
		]
	}}`

	client, _ := NewClient(ctx,
		WithAttributes(attributes),
		WithJsonFeatures(featuresJson),
	)

	result1 := client.EvalFeature(ctx, "childFlag")
	require.Equal(t, "success", result1.Value)
}

func TestConditionallyAppliesForceRuleBasedOnPrerequisiteTargeting(t *testing.T) {
	attributes := Attributes{
		"id":                  "123",
		"memberType":          "basic",
		"otherGatingProperty": "allow",
		"country":             "USA",
	}

	featuresJson := `
    {
	"parentFlag": {
		"defaultValue": "silver",
		"rules": [
			{
				"condition": {
					"country": "Canada"
				},
				"force": "red"
			},
			{
				"condition": {
					"country": {
						"$in": [
							"USA",
							"Mexico"
						]
					}
				},
				"force": "green"
			}
		]
	},
	"childFlag": {
		"defaultValue": "default",
		"rules": [
			{
				"parentConditions": [
					{
						"id": "parentFlag",
						"condition": {
							"value": "green"
						}
					}
				],
				"condition": {
					"otherGatingProperty": "allow"
				},
				"force": "dark mode"
			},
			{
				"condition": {
					"memberType": "basic"
				},
				"force": "light mode"
			}
		]
	}}`

	client, _ := NewClient(ctx,
		WithAttributes(attributes),
		WithJsonFeatures(featuresJson),
	)
	result := client.EvalFeature(ctx, "childFlag")
	require.Equal(t, "dark mode", result.Value)

	client, _ = client.WithAttributes(Attributes{
		"id":                  "123",
		"memberType":          "basic",
		"otherGatingProperty": "allow",
		"country":             "Canada",
	})

	result = client.EvalFeature(ctx, "childFlag")
	require.Equal(t, "light mode", result.Value)

	client, _ = client.WithAttributes(Attributes{
		"id":                  "123",
		"memberType":          "basic",
		"otherGatingProperty": "deny",
		"country":             "USA",
	})

	result = client.EvalFeature(ctx, "childFlag")
	require.Equal(t, "light mode", result.Value)
}

func TestConditionallyAppliesForceRuleBasedOnPrerequisiteJSONtargeting(t *testing.T) {
	attributes := Attributes{
		"id":         "123",
		"memberType": "basic",
		"country":    "USA",
	}

	featuresJson := `
    {
	"parentFlag": {
		"defaultValue": {
			"foo": true,
			"bar": {}
		},
		"rules": [
			{
				"condition": {
					"country": "Canada"
				},
				"force": {
					"foo": true,
					"bar": {
						"color": "red"
					}
				}
			},
			{
				"condition": {
					"country": {
						"$in": [
							"USA",
							"Mexico"
						]
					}
				},
				"force": {
					"foo": true,
					"bar": {
						"color": "green"
					}
				}
			}
		]
	},
	"childFlag": {
		"defaultValue": "default",
		"rules": [
			{
				"parentConditions": [
					{
						"id": "parentFlag",
						"condition": {
							"value.bar.color": "green"
						}
					}
				],
				"force": "dark mode"
			},
			{
				"condition": {
					"memberType": "basic"
				},
				"force": "light mode"
			}
		]
	},
	"childFlag2": {
		"defaultValue": "default",
		"rules": [
			{
				"parentConditions": [
					{
						"id": "parentFlag",
						"condition": {
							"value": {
								"$exists": true
							}
						}
					}
				],
				"force": "dark mode"
			},
			{
				"condition": {
					"memberType": "basic"
				},
				"force": "light mode"
			}
		]
	}}`

	client, _ := NewClient(ctx,
		WithAttributes(attributes),
		WithJsonFeatures(featuresJson))

	result := client.EvalFeature(ctx, "childFlag")
	require.Equal(t, "dark mode", result.Value)

	result = client.EvalFeature(ctx, "childFlag2")
	require.Equal(t, "dark mode", result.Value)

	client, _ = client.WithAttributes(Attributes{
		"id":                  "123",
		"memberType":          "basic",
		"otherGatingProperty": "allow",
		"country":             "Canada",
	})

	result = client.EvalFeature(ctx, "childFlag")
	require.Equal(t, "light mode", result.Value)
}

func TestReturnsNullWhenHittingPrerequisiteCycle(t *testing.T) {
	attributes := Attributes{
		"id":         "123",
		"memberType": "basic",
		"country":    "USA",
	}

	featuresJson := `
{
	"parentFlag": {
		"defaultValue": "silver",
		"rules": [
			{
				"parentConditions": [
					{
						"id": "childFlag",
						"condition": {
							"$not": {
								"value": "success"
							}
						}
					}
				],
				"force": null
			},
			{
				"condition": {
					"country": "Canada"
				},
				"force": "red"
			},
			{
				"condition": {
					"country": {
						"$in": [
							"USA",
							"Mexico"
						]
					}
				},
				"force": "green"
			}
		]
	},
	"childFlag": {
		"defaultValue": "default",
		"rules": [
			{
				"parentConditions": [
					{
						"id": "parentFlag",
						"condition": {
							"$not": {
								"value": "green"
							}
						}
					}
				],
				"force": null
			},
			{
				"condition": {
					"memberType": "basic"
				},
				"force": "success"
			}
		]
	}}`

	client, _ := NewClient(ctx,
		WithAttributes(attributes),
		WithJsonFeatures(featuresJson))

	result := client.EvalFeature(ctx, "childFlag")
	require.Nil(t, result.Value)
	require.Equal(t, CyclicPrerequisiteResultSource, result.Source)
}

func TestForcedFeatureOverridesRulesAndDefault(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "id": "foo"}]}
    }`

	client, _ := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithForcedFeatures(ForcedFeaturesMap{"feature": 99}))

	result := client.EvalFeature(ctx, "feature")
	require.Equal(t, 99, result.Value)
	require.Equal(t, OverrideResultSource, result.Source)
	require.True(t, result.On)
	// Forced override skips rules, so no rule id is reported.
	require.Equal(t, "", result.RuleId)
}

func TestForcedFeatureWorksForUnknownFeature(t *testing.T) {
	client, _ := NewClient(ctx,
		WithForcedFeatures(ForcedFeaturesMap{"missing": "forced"}))

	result := client.EvalFeature(ctx, "missing")
	require.Equal(t, "forced", result.Value)
	require.Equal(t, OverrideResultSource, result.Source)
}

// featureUsageSpyPlugin counts OnFeatureEvaluated calls for tests.
type featureUsageSpyPlugin struct{ featureEvaluated int }

func (p *featureUsageSpyPlugin) Init(*Client) error { return nil }
func (p *featureUsageSpyPlugin) Close() error       { return nil }
func (p *featureUsageSpyPlugin) OnExperimentViewed(context.Context, *Experiment, *ExperimentResult) {
}
func (p *featureUsageSpyPlugin) OnFeatureEvaluated(context.Context, string, *FeatureResult) {
	p.featureEvaluated++
}

func TestForcedFeatureDoesNotReportFeatureUsage(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "id": "foo"}]},
    "normal": {"defaultValue": 5}
    }`

	var calls int
	spy := &featureUsageSpyPlugin{}
	client, _ := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithForcedFeatures(ForcedFeaturesMap{"feature": 99}),
		WithPlugins(spy),
		WithFeatureUsageCallback(func(_ context.Context, _ string, _ *FeatureResult, _ any) {
			calls++
		}))

	// Forced (override) result must not be reported as feature usage on either
	// the callback or the plugin.
	res := client.EvalFeature(ctx, "feature")
	require.Equal(t, OverrideResultSource, res.Source)
	require.Equal(t, 0, calls)
	require.Equal(t, 0, spy.featureEvaluated)

	// A normal (non-forced) evaluation still reports usage on both.
	client.EvalFeature(ctx, "normal")
	require.Equal(t, 1, calls)
	require.Equal(t, 1, spy.featureEvaluated)
}

func TestForcedFeatureDoesNotAffectOtherFeatures(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "id": "foo"}]}
    }`

	client, _ := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithForcedFeatures(ForcedFeaturesMap{"other": true}))

	result := client.EvalFeature(ctx, "feature")
	require.Equal(t, float64(1), result.Value)
	require.Equal(t, ForceResultSource, result.Source)
}

func TestChildClientWithForcedFeatures(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "id": "foo"}]}
    }`

	parent, _ := NewClient(ctx, WithJsonFeatures(featuresJson))
	child, err := parent.WithForcedFeatures(ForcedFeaturesMap{"feature": 42})
	require.Nil(t, err)

	// Child sees the override.
	childResult := child.EvalFeature(ctx, "feature")
	require.Equal(t, 42, childResult.Value)
	require.Equal(t, OverrideResultSource, childResult.Source)

	// Parent is unaffected.
	parentResult := parent.EvalFeature(ctx, "feature")
	require.Equal(t, float64(1), parentResult.Value)
	require.Equal(t, ForceResultSource, parentResult.Source)
}

func TestChildForcedFeaturesMergeWithParent(t *testing.T) {
	featuresJson := `{
    "a": {"defaultValue": "da"},
    "b": {"defaultValue": "db"},
    "c": {"defaultValue": "dc"}
    }`

	parent, _ := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithForcedFeatures(ForcedFeaturesMap{"a": "A", "b": "B"}),
	)
	child, err := parent.WithForcedFeatures(ForcedFeaturesMap{"b": "B2", "c": "C"})
	require.Nil(t, err)

	// Child merges over the parent: a kept, b overridden, c added.
	require.Equal(t, "A", child.EvalFeature(ctx, "a").Value)
	require.Equal(t, "B2", child.EvalFeature(ctx, "b").Value)
	require.Equal(t, "C", child.EvalFeature(ctx, "c").Value)

	// Parent is unaffected by the child merge.
	require.Equal(t, "A", parent.EvalFeature(ctx, "a").Value)
	require.Equal(t, "B", parent.EvalFeature(ctx, "b").Value)
	require.Equal(t, "dc", parent.EvalFeature(ctx, "c").Value)
}
