package growthbook

import "github.com/growthbook/growthbook-golang/internal/condition"

// ContextualBanditContext is one leaf of a contextual bandit definition: a
// targeting condition and the variation weights to use when it matches.
type ContextualBanditContext struct {
	LeafId    int            `json:"leafId"`
	Condition condition.Base `json:"condition"`
	Weights   []float64      `json:"weights"`
}

// ContextualBanditDefinition holds the per-context variation weights for one
// contextual bandit, as served in the SDK payload.
type ContextualBanditDefinition struct {
	BanditVersion *int                      `json:"banditVersion"`
	Contexts      []ContextualBanditContext `json:"contexts"`
}

// ContextualBanditDefinitions maps bandit refs to their definitions.
type ContextualBanditDefinitions map[string]ContextualBanditDefinition

// CBContext is the contextual bandit context an assignment was made with,
// mirroring the JS SDK type of the same name.
type CBContext struct {
	LeafId           int       `json:"leafId"`
	VariationWeights []float64 `json:"variationWeights"`
	BanditVersion    *int      `json:"banditVersion,omitempty"`
}

const contextualBanditFallbackLeafId = -1

// buildContextualBanditExperiment applies a bandit definition to exp: the
// first leaf whose condition passes supplies the variation weights; with no
// matching leaf the aggregate weights stay and the exposure is attributed to
// the fallback leaf. A missing definition leaves exp untouched.
func (e *evaluator) buildContextualBanditExperiment(exp *Experiment, ref string, featureId string) {
	def, ok := e.contextualBandits[ref]
	if !ok {
		e.client.logger.DebugContext(e.ctx, "Contextual bandit ref not found in payload, using aggregate weights",
			"id", featureId, "contextualBanditRef", ref)
		return
	}

	for _, leaf := range def.Contexts {
		if !leaf.Condition.Eval(e.client.attributes, e.savedGroups) {
			continue
		}
		exp.Weights = leaf.Weights
		exp.ContextualBandit = &CBContext{
			LeafId:           leaf.LeafId,
			VariationWeights: leaf.Weights,
			BanditVersion:    def.BanditVersion,
		}
		return
	}

	e.client.logger.DebugContext(e.ctx, "Contextual bandit: no matching leaf, using fallback weights",
		"id", featureId, "contextualBanditRef", ref)
	weights := exp.Weights
	if len(weights) == 0 {
		weights = getEqualWeights(len(exp.Variations))
	}
	exp.ContextualBandit = &CBContext{
		LeafId:           contextualBanditFallbackLeafId,
		VariationWeights: weights,
		BanditVersion:    def.BanditVersion,
	}
}
