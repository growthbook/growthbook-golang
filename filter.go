package growthbook

// Filter represents a filter condition for experiment mutual
// exclusion.
type Filter struct {
	Attribute   string  `json:"attribute,omitempty"`
	Seed        string  `json:"seed,omitempty"`
	HashVersion int     `json:"hashVersion,omitempty"`
	Ranges      []Range `json:"ranges,omitempty"`
}
