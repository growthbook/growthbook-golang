package growthbook

import (
	"fmt"
	"testing"

	. "github.com/franela/goblin"
)

var regressionTests = map[int]func(g *G){
	1: issue1, // https://github.com/growthbook/growthbook-golang/issues/1
	5: issue5, // https://github.com/growthbook/growthbook-golang/issues/5
}

func TestRegressions(t *testing.T) {
	g := Goblin(t)
	g.Describe("regression tests", func() {
		for itest, test := range regressionTests {
			g.It(fmt.Sprintf("issue #%d", itest), func() {
				test(g)
			})
		}
	})
}

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

func issue1Like(g *G, attrs Attributes) {
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
	g.Assert(value).Equal(expectedValue)
}

func issue1(g *G) {
	// Check with slice value for attribute.
	attrs := Attributes{
		"id":                  "user-employee-123456789",
		"loggedIn":            true,
		"employee":            true,
		"country":             "france",
		"dietaryRestrictions": []string{"gluten_free"},
	}

	issue1Like(g, attrs)
}

func issue5(g *G) {
	// Check with array value for attribute.
	attrs := Attributes{
		"id":                  "user-employee-123456789",
		"loggedIn":            true,
		"employee":            true,
		"country":             "france",
		"dietaryRestrictions": [1]string{"gluten_free"},
	}

	issue1Like(g, attrs)
}
