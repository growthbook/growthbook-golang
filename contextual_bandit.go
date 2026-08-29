package growthbook

import (
	"encoding/json"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

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

// UnmarshalJSON drops malformed contexts instead of erroring, so one bad
// leaf does not discard a definition's parseable ones.
func (d *ContextualBanditDefinition) UnmarshalJSON(data []byte) error {
	var raw struct {
		BanditVersion *int              `json:"banditVersion"`
		Contexts      []json.RawMessage `json:"contexts"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.BanditVersion = raw.BanditVersion
	d.Contexts = nil
	for _, rawCtx := range raw.Contexts {
		var c ContextualBanditContext
		if err := json.Unmarshal(rawCtx, &c); err != nil {
			continue
		}
		d.Contexts = append(d.Contexts, c)
	}
	return nil
}

// ContextualBanditDefinitions maps bandit refs to their definitions.
type ContextualBanditDefinitions map[string]ContextualBanditDefinition

// UnmarshalJSON decodes leniently, dropping malformed definitions instead of
// erroring: a bandit blob the SDK cannot parse (e.g. a future schema) must
// not block the feature update it arrived with. Affected rules fall back to
// their aggregate weights.
func (defs *ContextualBanditDefinitions) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		*defs = nil
		return nil
	}
	out := make(ContextualBanditDefinitions, len(raw))
	for ref, rawDef := range raw {
		var def ContextualBanditDefinition
		if err := json.Unmarshal(rawDef, &def); err != nil {
			continue
		}
		out[ref] = def
	}
	*defs = out
	return nil
}

// CBContext is the contextual bandit context an assignment was made with.
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
		// Sanitize so the reported weights always equal the weights the
		// assignment uses — bandit analysis reweights by these propensities.
		weights := e.client.effectiveWeights(len(exp.Variations), leaf.Weights)
		exp.Weights = weights
		exp.ContextualBandit = &CBContext{
			LeafId:           leaf.LeafId,
			VariationWeights: weights,
			BanditVersion:    def.BanditVersion,
		}
		return
	}

	e.client.logger.DebugContext(e.ctx, "Contextual bandit: no matching leaf, using fallback weights",
		"id", featureId, "contextualBanditRef", ref)
	exp.ContextualBandit = &CBContext{
		LeafId:           contextualBanditFallbackLeafId,
		VariationWeights: e.client.effectiveWeights(len(exp.Variations), exp.Weights),
		BanditVersion:    def.BanditVersion,
	}
}
