package growthbook

// Filter represents a filter condition for experiment mutual
// exclusion.
type Filter struct {
	Seed        string        `json:"seed"`
	Ranges      []BucketRange `json:"ranges"`
	Attribute   string        `json:"attribute"`
	HashVersion int           `json:"hashVersion"`
}
