package growthbook

import (
	"encoding/json"
	"errors"
)

// Result records the result of running an Experiment given a specific
// Context.
type Result struct {
	Value         FeatureValue
	VariationID   int
	Key           string
	Name          string
	Bucket        *float64
	Passthrough   bool
	InExperiment  bool
	HashUsed      bool
	HashAttribute string
	HashValue     string
	FeatureID     string
}

// UnmarshalJSON deserializes experiment result data from JSON, with
// custom conversion of the hash value field.
func (r *Result) UnmarshalJSON(data []byte) error {
	type Alias Result
	tmp := &struct {
		*Alias
		HashValue interface{}
	}{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	hashValue, ok := convertHashValue(tmp.HashValue)
	if !ok {
		return errors.New("invalid JSON type for hashValue")
	}
	r.Value = tmp.Value
	r.VariationID = tmp.VariationID
	r.Key = tmp.Key
	r.Name = tmp.Name
	r.Bucket = tmp.Bucket
	r.Passthrough = tmp.Passthrough
	r.InExperiment = tmp.InExperiment
	r.HashUsed = tmp.HashUsed
	r.HashAttribute = tmp.HashAttribute
	r.HashValue = hashValue
	r.FeatureID = tmp.FeatureID
	return nil
}
