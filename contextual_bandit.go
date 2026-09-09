package growthbook

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	// raw is the original payload form, retained only while malformed is set:
	// a malformed context has no truthful field representation, so it
	// serializes as its original junk instead of a valid-looking leaf.
	raw json.RawMessage
}

// MarshalJSON emits the context's current fields, so values modified after
// decoding serialize what evaluation would use. The one exception is a
// malformed context: evaluation ignores its fields (leaf selection aborts at
// it), so serialization preserves the original junk — emitting fields would
// launder it into a valid catch-all leaf and change routing on re-decode.
func (c ContextualBanditContext) MarshalJSON() ([]byte, error) {
	if c.malformed {
		return c.raw, nil
	}
	type alias ContextualBanditContext
	return json.Marshal(alias(c))
}

// UnmarshalJSON never fails: field-level junk zeroes the field, and a
// non-object context (or one with an unparseable condition) is kept as a
// malformed marker instead of being dropped, so routing order is preserved.
func (c *ContextualBanditContext) UnmarshalJSON(data []byte) error {
	*c = ContextualBanditContext{raw: append(json.RawMessage(nil), data...)}
	defer func() {
		if !c.malformed {
			c.raw = nil
		}
	}()
	var raw struct {
		LeafId    json.RawMessage `json:"leafId"`
		Condition json.RawMessage `json:"condition"`
		Weights   json.RawMessage `json:"weights"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		c.malformed = true
		return nil
	}
	// Guard against JSON null explicitly: json.Unmarshal treats null as a
	// successful no-op, which would turn "leafId": null into leaf 0.
	if raw.LeafId != nil && string(raw.LeafId) != "null" {
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

// MarshalJSON emits the definition's current fields, so values modified
// after decoding serialize what evaluation would use. A falsy definition is
// the exception: evaluation treats it as missing regardless of its fields,
// so it serializes as its canonical falsy form, null — emitting fields would
// turn a dangling ref into a fallback definition on re-decode.
func (d ContextualBanditDefinition) MarshalJSON() ([]byte, error) {
	if d.falsy {
		return []byte("null"), nil
	}
	type alias ContextualBanditDefinition
	return json.Marshal(alias(d))
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
	// Guard against JSON null explicitly: json.Unmarshal treats null as a
	// successful no-op, which would turn "banditVersion": null into version 0.
	if raw.BanditVersion != nil && string(raw.BanditVersion) != "null" {
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

// ParseContextualBandits strictly parses a contextual bandit definitions
// blob for manual configuration: unlike the tolerant UnmarshalJSON — which
// exists for the payload-ingestion boundary, where a bandit blob must never
// block the feature update it arrived with — a top-level shape that is not a
// JSON object is reported as an error instead of degrading to an empty map.
// Definition- and context-level junk still degrades per entry at evaluation
// time, exactly as payload-served definitions do.
func ParseContextualBandits(data []byte) (ContextualBanditDefinitions, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("contextual bandit definitions must be a JSON object keyed by ref: %w", err)
	}
	if probe == nil {
		// json.Unmarshal accepts null into a map as a successful no-op.
		return nil, fmt.Errorf("contextual bandit definitions must be a JSON object keyed by ref, got null")
	}
	var defs ContextualBanditDefinitions
	_ = json.Unmarshal(data, &defs) // the tolerant decoder cannot fail on an object
	return defs, nil
}

// UnmarshalJSON decodes leniently: a bandit blob the SDK cannot parse (e.g.
// a future schema) must not block the feature update it arrived with.
// Definition- and context-level junk degrades per entry at evaluation time;
// map-level junk yields an empty map, so every ref dangles (debug-logged at
// use). Manual callers who want a top-level shape error instead should use
// ParseContextualBandits.
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
