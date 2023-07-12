package growthbook

import (
	"encoding/json"
	"reflect"
	"testing"
)

const issue1FeaturesJson = `{
	"banner_text": {
		"defaultValue": "Welcome to Acme Donuts!",
		"rules": [
			{
				"condition": { "country": "france" },
				"force": "Bienvenue au Beignets Acme !"
			},
			{
				"condition": { "country": "spain" },
				"force": "¡Bienvenidos y bienvenidas a Donas Acme!"
			}
		]
	},
	"dark_mode": {
		"defaultValue": false,
		"rules": [
			{
				"condition": { "loggedIn": true },
				"force": true,
				"coverage": 0.5,
				"hashAttribute": "id"
			}
		]
	},
	"donut_price": {
		"defaultValue": 2.5,
		"rules": [{ "condition": { "employee": true }, "force": 0 }]
	},
	"meal_overrides_gluten_free": {
		"defaultValue": {
			"meal_type": "standard",
			"dessert": "Strawberry Cheesecake"
		},
		"rules": [
			{
				"condition": {
					"dietaryRestrictions": { "$elemMatch": { "$eq": "gluten_free" } }
				},
				"force": { "meal_type": "gf", "dessert": "French Vanilla Ice Cream" }
			}
		]
	}
}`

const issue1AttributesJson = `{"employee":false,"loggedIn":true,"dietaryRestrictions":["gluten_free"]}`

const issue1ContextJson = `{
  "attributes": {"employee":false,"loggedIn":true,"dietaryRestrictions":["gluten_free"]},
  "features": {
		"banner_text": {
			"defaultValue": "Welcome to Acme Donuts!",
			"rules": [
				{
					"condition": { "country": "france" },
					"force": "Bienvenue au Beignets Acme !"
				},
				{
					"condition": { "country": "spain" },
					"force": "¡Bienvenidos y bienvenidas a Donas Acme!"
				}
			]
		},
		"dark_mode": {
			"defaultValue": false,
			"rules": [
				{
					"condition": { "loggedIn": true },
					"force": true,
					"coverage": 0.5,
					"hashAttribute": "id"
				}
			]
		},
		"donut_price": {
			"defaultValue": 2.5,
			"rules": [{ "condition": { "employee": true }, "force": 0 }]
		},
		"meal_overrides_gluten_free": {
			"defaultValue": {
				"meal_type": "standard",
				"dessert": "Strawberry Cheesecake"
			},
			"rules": [
				{
					"condition": {
						"dietaryRestrictions": { "$elemMatch": { "$eq": "gluten_free" } }
					},
					"force": { "meal_type": "gf", "dessert": "French Vanilla Ice Cream" }
				}
			]
		}
  }
}`

const issue1ExpectedJson = `{ "meal_type": "gf", "dessert": "French Vanilla Ice Cream" }`

func TestIssue1(t *testing.T) {
	// Check with slice value for attribute.
	attrs := Attributes{
		"id":                  "user-employee-123456789",
		"loggedIn":            true,
		"employee":            true,
		"country":             "france",
		"dietaryRestrictions": []string{"gluten_free"},
	}

	features := FeatureMap{}
	err := json.Unmarshal([]byte(issue1FeaturesJson), &features)
	if err != nil {
		t.Errorf("failed to parse features JSON: %s", issue1FeaturesJson)
	}

	client := NewClient(nil).WithFeatures(features)

	value := client.EvalFeature("meal_overrides_gluten_free", attrs).Value

	expectedValue := map[string]interface{}{
		"meal_type": "gf",
		"dessert":   "French Vanilla Ice Cream",
	}

	if !reflect.DeepEqual(value, expectedValue) {
		t.Errorf("unexpected value: %v", value)
	}
}

func TestIssue5(t *testing.T) {
	// Check with array value for attribute.
	attrs := Attributes{
		"id":                  "user-employee-123456789",
		"loggedIn":            true,
		"employee":            true,
		"country":             "france",
		"dietaryRestrictions": [1]string{"gluten_free"},
	}

	features := FeatureMap{}
	err := json.Unmarshal([]byte(issue1FeaturesJson), &features)
	if err != nil {
		t.Errorf("failed to parse features JSON: %s", issue1FeaturesJson)
	}

	client := NewClient(nil).WithFeatures(features)

	value := client.EvalFeature("meal_overrides_gluten_free", attrs).Value

	expectedValue := map[string]interface{}{
		"meal_type": "gf",
		"dessert":   "French Vanilla Ice Cream",
	}

	if !reflect.DeepEqual(value, expectedValue) {
		t.Errorf("unexpected value: %v", value)
	}
}

const numericComparisonsJson = `{
  "donut_price": {
    "defaultValue": 2.5,
    "rules": [
      {
        "condition": {
          "bonus_scheme": 2
        },
        "force": 1.0
      }
    ]
  },
  "donut_rating": {
    "defaultValue": 4,
    "rules": [
      {
        "condition": {
          "bonus_scheme": 1
        },
        "force": 1
      }
    ]
  }

}
`

func TestNumericComparisons(t *testing.T) {
	features := FeatureMap{}
	err := json.Unmarshal([]byte(numericComparisonsJson), &features)
	if err != nil {
		t.Errorf("failed to parse features JSON: %s", numericComparisonsJson)
	}

	attrs := Attributes{"bonus_scheme": 2}

	client := NewClient(nil).WithFeatures(features)

	value1 := client.EvalFeature("donut_price", attrs).Value
	if value1 != 1.0 {
		t.Errorf("unexpected value: %v", value1)
	}
	value2 := client.EvalFeature("donut_rating", attrs).Value
	if value2 != 4.0 {
		t.Errorf("unexpected value: %v", value2)
	}
}
