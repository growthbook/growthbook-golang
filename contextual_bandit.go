package growthbook

import "github.com/growthbook/growthbook-golang/internal/condition"

// contextualBanditFallbackLeafId marks a result where the bandit definition was
// found but no context (leaf) matched the user.
const contextualBanditFallbackLeafId = -1

// ContextualBanditContext is a single leaf of a contextual bandit: a targeting
// condition and the backend-computed variation weights to use when it matches.
// A nil/empty condition matches everyone (catch-all leaf).
type ContextualBanditContext struct {
	LeafId    *int           `json:"leafId"`
	Condition condition.Base `json:"condition"`
	Weights   []float64      `json:"weights"`
}

// ContextualBanditDefinition is a bandit as delivered in the feature payload:
// a version and an ordered list of contexts evaluated top-to-bottom
// (first match wins).
type ContextualBanditDefinition struct {
	BanditVersion *int                      `json:"banditVersion"`
	Contexts      []ContextualBanditContext `json:"contexts"`
}

// ContextualBanditsMap maps a bandit ref (as referenced by a feature rule) to
// its definition.
type ContextualBanditsMap map[string]*ContextualBanditDefinition

// contextualBandit is the outcome of applying a bandit during evaluation,
// surfaced on the experiment result when the user has a real exposure.
type contextualBandit struct {
	leafId           *int
	variationWeights []float64
	banditVersion    *int
}

// WithContextualBandits sets contextual bandit definitions directly (usually
// they arrive with the feature payload).
func WithContextualBandits(bandits ContextualBanditsMap) ClientOption {
	return func(c *Client) error {
		c.data.contextualBandits = bandits
		return nil
	}
}
