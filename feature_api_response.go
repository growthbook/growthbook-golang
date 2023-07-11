package growthbook

import (
	"encoding/json"
	"time"
)

const dateLayout = "2006-01-02T15:04:05.000Z"

type FeatureAPIResponse struct {
	Status            int                 `json:"status"`
	Features          map[string]*Feature `json:"features"`
	DateUpdated       time.Time           `json:"dateUpdated,omitempty"`
	EncryptedFeatures string              `json:"encryptedFeatures,omitempty"`
}

// MarshalJSON serializes feature API response data to JSON, with
// custom conversion of the DateUpdated field.
func (r FeatureAPIResponse) MarshalJSON() ([]byte, error) {
	type Alias FeatureAPIResponse
	tmp := &struct {
		*Alias
		DateUpdated string `json:"dateUpdated"`
	}{
		Alias:       (*Alias)(&r),
		DateUpdated: r.DateUpdated.Format(dateLayout),
	}
	return json.Marshal(tmp)
}

// UnmarshalJSON deserializes feature API response data from JSON,
// with custom conversion of the DateUpdated field.
func (r *FeatureAPIResponse) UnmarshalJSON(data []byte) error {
	type Alias FeatureAPIResponse
	tmp := &struct {
		*Alias
		DateUpdated string `json:"dateUpdated"`
	}{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	r.Status = tmp.Status
	r.Features = tmp.Features
	if tmp.DateUpdated != "" {
		dateUpdated, err := time.Parse(dateLayout, tmp.DateUpdated)
		if err != nil {
			return err
		}
		r.DateUpdated = dateUpdated
	}
	r.EncryptedFeatures = tmp.EncryptedFeatures
	return nil
}
