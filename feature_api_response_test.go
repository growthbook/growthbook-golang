package growthbook

import (
	"encoding/json"
	"testing"
)

const jsonData = `{
  "status": 200,
  "features": {
    "banner_text": {
      "defaultValue": "Welcome to Acme Donuts!",
      "rules": [
        {
          "condition": {
            "country": "france"
          },
          "force": "Bienvenue au Beignets Acme !"
        },
        {
          "condition": {
            "country": "spain"
          },
          "force": "¡Bienvenidos y bienvenidas a Donas Acme!"
        }
      ]
    },
    "dark_mode": {
      "defaultValue": false,
      "rules": [
        {
          "condition": {
            "loggedIn": true
          },
          "force": true,
          "coverage": 0.5,
          "hashAttribute": "id"
        }
      ]
    },
    "donut_price": {
      "defaultValue": 2.5,
      "rules": [
        {
          "condition": {
            "employee": true
          },
          "force": 0
        }
      ]
    },
    "meal_overrides_gluten_free": {
      "defaultValue": {
        "meal_type": "standard",
        "dessert": "Strawberry Cheesecake"
      },
      "rules": [
        {
          "condition": {
            "dietaryRestrictions": {
              "$elemMatch": {
                "$eq": "gluten_free"
              }
            }
          },
          "force": {
            "meal_type": "gf",
            "dessert": "French Vanilla Ice Cream"
          }
        }
      ]
    },
    "app_name": {
      "defaultValue": "(unknown)",
      "rules": [
        {
          "condition": {
            "version": {
              "$vgte": "1.0.0",
              "$vlt": "2.0.0"
            }
          },
          "force": "Albatross"
        },
        {
          "condition": {
            "version": {
              "$vgte": "2.0.0",
              "$vlt": "3.0.0"
            }
          },
          "force": "Badger"
        },
        {
          "condition": {
            "version": {
              "$vgte": "3.0.0",
              "$vlt": "4.0.0"
            }
          },
          "force": "Capybara"
        }
      ]
    }
  },
  "dateUpdated": "2023-06-27T18:10:13.378Z"
}`

func TestAPIResponseParsing(t *testing.T) {
	apiResponse := FeatureAPIResponse{}
	err := json.Unmarshal([]byte(jsonData), &apiResponse)
	if err != nil {
		t.Errorf("failed to parse API response data")
	}
}
