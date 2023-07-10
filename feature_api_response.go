package growthbook

import (
	"encoding/json"
	"errors"
	"time"
)

const dateLayout = "2006-01-02T15:04:05.000Z"

type FeatureAPIResponse struct {
	Status            int                 `json:"status"`
	Features          map[string]*Feature `json:"features"`
	DateUpdated       time.Time           `json:"dateUpdated"`
	EncryptedFeatures string              `json:"encryptedFeatures"`
}

// Implement normal JSON marshalling interfaces for convenience.

func (r *FeatureAPIResponse) MarshalJSON() ([]byte, error) {
	type Alias FeatureAPIResponse
	return json.Marshal(&struct {
		*Alias
		DateUpdated string `json:"dateUpdated"`
	}{
		Alias:       (*Alias)(r),
		DateUpdated: r.DateUpdated.Format(dateLayout),
	})
}

func (r *FeatureAPIResponse) UnmarshalJSON(data []byte) error {
	parsed := ParseFeatureAPIResponse(data)
	if parsed == nil {
		return errors.New("failed to parse feature API response")
	}
	r.Features = parsed.Features
	r.DateUpdated = parsed.DateUpdated
	r.EncryptedFeatures = parsed.EncryptedFeatures
	return nil
}

// ParseFeature creates a single Feature value from raw JSON input.
func ParseFeatureAPIResponse(data []byte) *FeatureAPIResponse {
	dict := make(map[string]interface{})
	err := json.Unmarshal(data, &dict)
	if err != nil {
		logError("Failed parsing JSON input", "FeatureAPIResponse")
		return nil
	}
	return BuildFeatureAPIResponse(dict)
}

// BuildFeatureAPIResponse creates a FeatureAPIResponse value from a
// generic JSON value.
func BuildFeatureAPIResponse(dict map[string]interface{}) *FeatureAPIResponse {
	apiResponse := FeatureAPIResponse{}
	for k, v := range dict {
		switch k {
		case "status":
			status, ok := jsonInt(v, "FeatureAPIResponse", "status")
			if !ok {
				return nil
			}
			apiResponse.Status = status
		case "features":
			apiResponse.Features = BuildFeatures(v)
		case "dateUpdated":
			dateUpdated, ok := jsonString(v, "FeatureAPIResponse", "dateUpdated")
			if !ok {
				return nil
			}
			var err error
			apiResponse.DateUpdated, err = time.Parse(dateLayout, dateUpdated)
			if err != nil {
				return nil
			}
		case "encryptedFeatures":
			encryptedFeatures, ok := jsonString(v, "FeatureAPIResponse", "encryptedFeatures")
			if !ok {
				return nil
			}
			apiResponse.EncryptedFeatures = encryptedFeatures
		default:
			logWarn("Unknown key in JSON data", "FeatureAPIResponse", k)
		}
	}
	return &apiResponse
}
