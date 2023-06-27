package growthbook

import "encoding/json"

type FeatureAPIResponse struct {
	Features          map[string]*Feature `json:"features"`
	DateUpdated       string              `json:"dateUpdated"`
	EncryptedFeatures string              `json:"encryptedFeatures"`
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
		case "features":
			apiResponse.Features = BuildFeatures(v)
		case "dateUpdated":
			dateUpdated, ok := jsonString(v, "FeatureAPIResponse", "dateUpdated")
			if !ok {
				return nil
			}
			apiResponse.DateUpdated = dateUpdated
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
