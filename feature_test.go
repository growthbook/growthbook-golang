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
		WithJsonFeatures(featuresJson))

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
