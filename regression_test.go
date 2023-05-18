package growthbook

import (
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

	features := ParseFeatureMap([]byte(issue1FeaturesJson))

	context := NewContext().
		WithFeatures(features).
		WithAttributes(attrs)

	gb := New(context)

	value := gb.Feature("meal_overrides_gluten_free").Value

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

	features := ParseFeatureMap([]byte(issue1FeaturesJson))

	context := NewContext().
		WithFeatures(features).
		WithAttributes(attrs)

	gb := New(context)

	value := gb.Feature("meal_overrides_gluten_free").Value

	expectedValue := map[string]interface{}{
		"meal_type": "gf",
		"dessert":   "French Vanilla Ice Cream",
	}

	if !reflect.DeepEqual(value, expectedValue) {
		t.Errorf("unexpected value: %v", value)
	}
}

func TestNilContext(t *testing.T) {
	// Check that there's no problem using a nil context.
	var nilContext *Context
	gbTest := New(nilContext)

	if !gbTest.Enabled() {
		t.Errorf("expected gbTest.enabled to be true")
	}
}
