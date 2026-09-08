package growthbook

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

// ContextualBanditContext is one leaf of a contextual bandit definition: a
// targeting condition and the variation weights to use when it matches. An
// absent condition is a catch-all leaf.
type ContextualBanditContext struct {
	// LeafId is nil when the payload's leafId is absent or not an integer;
	// such a leaf cannot be attributed and demotes to the fallback.
	LeafId    *int           `json:"leafId"`
	Condition condition.Base `json:"condition"`
	// Weights is nil when the payload's weights are absent or type-malformed.
	Weights []float64 `json:"weights"`

	// malformed marks a context that was not a JSON object, or whose
	// condition could not be parsed. It is kept in place rather than dropped:
	// routing cannot know whether it would have matched, so reaching it
	// aborts leaf selection (Python SDK parity).
	malformed bool
}

// UnmarshalJSON never fails: field-level junk zeroes the field, and a
// non-object context (or one with an unparseable condition) is kept as a
// malformed marker instead of being dropped, so routing order is preserved.
func (c *ContextualBanditContext) UnmarshalJSON(data []byte) error {
	*c = ContextualBanditContext{}
	var raw struct {
		LeafId    json.RawMessage `json:"leafId"`
		Condition json.RawMessage `json:"condition"`
		Weights   json.RawMessage `json:"weights"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		c.malformed = true
		return nil
	}
	if raw.LeafId != nil {
		var id int
		if err := json.Unmarshal(raw.LeafId, &id); err == nil {
			c.LeafId = &id
		}
	}
	if raw.Condition != nil && string(raw.Condition) != "null" {
		if err := json.Unmarshal(raw.Condition, &c.Condition); err != nil {
			c.malformed = true
		}
	}
	if raw.Weights != nil {
		// A partially numeric array must not survive as a prefix: reject the
		// vector wholesale unless every element decodes.
		if err := json.Unmarshal(raw.Weights, &c.Weights); err != nil {
			c.Weights = nil
		}
	}
	return nil
}

// ContextualBanditDefinition holds the per-context variation weights for one
// contextual bandit, as served in the SDK payload.
type ContextualBanditDefinition struct {
	BanditVersion *int                      `json:"banditVersion"`
	Contexts      []ContextualBanditContext `json:"contexts"`

	// falsy marks a definition that was JSON null, false, 0, or "" — the JS
	// SDK's `!cbDefinition` check treats those like a missing definition
	// (dangling ref, no bandit metadata), where truthy junk takes the
	// fallback-leaf path.
	falsy bool
}

// UnmarshalJSON never fails: a junk banditVersion is omitted without
// dropping the definition, junk contexts decode to none, and a non-object
// definition is kept, distinguishing JS-falsy values from truthy junk.
func (d *ContextualBanditDefinition) UnmarshalJSON(data []byte) error {
	*d = ContextualBanditDefinition{}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		// Not an object. JS-falsy values behave like a missing definition
		// (`!cbDefinition`); truthy junk keeps an empty definition, which
		// takes the fallback-leaf path.
		var v any
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil
		}
		switch val := v.(type) {
		case nil:
			d.falsy = true
		case bool:
			d.falsy = !val
		case float64:
			d.falsy = val == 0
		case string:
			d.falsy = val == ""
		}
		return nil
	}
	var raw struct {
		BanditVersion json.RawMessage `json:"banditVersion"`
		Contexts      json.RawMessage `json:"contexts"`
	}
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil
	}
	if raw.BanditVersion != nil {
		var v int
		if err := json.Unmarshal(raw.BanditVersion, &v); err == nil {
			d.BanditVersion = &v
		}
	}
	if raw.Contexts != nil {
		// Non-array contexts decode to none; per-context junk is handled by
		// the context decoder, which never fails.
		if err := json.Unmarshal(raw.Contexts, &d.Contexts); err != nil {
			d.Contexts = nil
		}
	}
	return nil
}

// ContextualBanditDefinitions maps bandit refs to their definitions.
type ContextualBanditDefinitions map[string]ContextualBanditDefinition

// UnmarshalJSON decodes leniently: a bandit blob the SDK cannot parse (e.g.
// a future schema) must not block the feature update it arrived with.
// Definition- and context-level junk degrades per entry at evaluation time;
// map-level junk yields an empty map, so every ref dangles (debug-logged at
// use).
func (defs *ContextualBanditDefinitions) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		*defs = ContextualBanditDefinitions{}
		return nil
	}
	out := make(ContextualBanditDefinitions, len(raw))
	for ref, rawDef := range raw {
		var def ContextualBanditDefinition
		// The definition decoder never fails; junk degrades per entry.
		_ = json.Unmarshal(rawDef, &def)
		out[ref] = def
	}
	*defs = out
	return nil
}

// ContextualBanditAssignment is what a contextual bandit assignment was
// made with: the chosen leaf (or the fallback -1) and the variation weights
// bucketing used. Distinct from ContextualBanditContext, which is a payload
// leaf (a targeting condition plus candidate weights).
type ContextualBanditAssignment struct {
	LeafId           int       `json:"leafId"`
	VariationWeights []float64 `json:"variationWeights"`
	BanditVersion    *int      `json:"banditVersion,omitempty"`
}

const contextualBanditFallbackLeafId = -1

// clonedBanditVersion detaches a bandit version from its owner: assignments
// must not share the pointer with the client-wide definitions map, and
// results must not share it with the experiment's assignment — a consumer
// writing through one must never corrupt what another observes.
func clonedBanditVersion(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// buildContextualBanditExperiment applies a bandit definition to exp: the
// first leaf whose condition passes supplies the variation weights; with no
// matching (valid) leaf the aggregate weights stay and the exposure is
// attributed to the fallback leaf. A missing or JS-falsy definition leaves
// exp untouched — a plain experiment with no bandit metadata.
func (e *evaluator) buildContextualBanditExperiment(exp *Experiment, ref string, featureId string) {
	def, ok := e.contextualBandits[ref]
	if !ok || def.falsy {
		e.client.logger.DebugContext(e.ctx, "Contextual bandit ref not found in payload, using aggregate weights",
			"id", featureId, "contextualBanditRef", ref)
		return
	}

	leaf := e.selectContextualBanditLeaf(def.Contexts, featureId, ref)
	if leaf != nil && (leaf.LeafId == nil || !isValidWeightVector(leaf.Weights, len(exp.Variations))) {
		// Demote rather than repair: substituting weights would report a
		// leaf whose propensities differ from the vector bucketing uses,
		// and keeping its leafId would attribute an assignment the leaf's
		// weights did not produce.
		e.client.logger.DebugContext(e.ctx, "Contextual bandit: matched leaf is malformed, using fallback weights",
			"id", featureId, "contextualBanditRef", ref)
		leaf = nil
	}

	if leaf != nil {
		// Clone: payload-owned slices must never alias into experiments and
		// results handed to callbacks and subscribers.
		weights := slices.Clone(leaf.Weights)
		exp.Weights = weights
		exp.ContextualBandit = &ContextualBanditAssignment{
			LeafId:           *leaf.LeafId,
			VariationWeights: weights,
			BanditVersion:    clonedBanditVersion(def.BanditVersion),
		}
		return
	}

	e.client.logger.DebugContext(e.ctx, "Contextual bandit: no matching leaf, using fallback weights",
		"id", featureId, "contextualBanditRef", ref)
	exp.ContextualBandit = &ContextualBanditAssignment{
		LeafId:           contextualBanditFallbackLeafId,
		VariationWeights: slices.Clone(normalizedWeights(len(exp.Variations), exp.Weights, e.client.logger)),
		BanditVersion:    clonedBanditVersion(def.BanditVersion),
	}
}

// selectContextualBanditLeaf returns the first leaf whose condition matches
// the client's attributes. Reaching a malformed context aborts selection —
// evaluation cannot know whether it would have matched, so later leaves must
// not be consulted (Python SDK parity: a failed leaf lookup falls back).
//
// "Malformed" here means structurally unusable: a non-object context or an
// unparseable condition. A condition that parses but carries semantic junk
// (an unknown operator, `$in` with a non-array argument, an invalid regex)
// deliberately evaluates false and routing continues — that is how the JS
// and Python SDKs evaluate the same payload, and diverging here would assign
// the same user different variations across SDKs.
func (e *evaluator) selectContextualBanditLeaf(contexts []ContextualBanditContext, featureId string, ref string) *ContextualBanditContext {
	for i := range contexts {
		leaf := &contexts[i]
		if leaf.malformed {
			e.client.logger.DebugContext(e.ctx, "Contextual bandit: leaf selection stopped at a malformed context",
				"id", featureId, "contextualBanditRef", ref)
			return nil
		}
		if leaf.Condition.Eval(e.client.attributes, e.savedGroups) {
			return leaf
		}
	}
	return nil
}
