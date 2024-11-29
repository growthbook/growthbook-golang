package growthbook

import "github.com/growthbook/growthbook-golang/internal/condition"

type ParentCondition struct {
	Id        string         `json:"id"`
	Condition condition.Base `json:"condition"`
	Gate      bool           `json:"gate"`
}
