package growthbook

import "encoding/json"

const interleaveFeaturePrefix = "$interleave:"

// InterleaveRanker is one ranker of an interleaving experiment: its key, which
// is the variation reported on exposures, and the config your ranking code
// reads.
type InterleaveRanker struct {
	Key    string         `json:"key"`
	Config map[string]any `json:"config"`
}

// DecodeConfig decodes the ranker's config into v, a pointer to your own
// config type, the way json.Unmarshal would.
func (r InterleaveRanker) DecodeConfig(v any) error {
	data, err := json.Marshal(r.Config)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// InterleavingDefinition is an interleaving experiment as the SDK payload
// carries it.
type InterleavingDefinition struct {
	TrackingKey   string             `json:"trackingKey"`
	HashAttribute string             `json:"hashAttribute"`
	Seed          string             `json:"seed"`
	Variations    []InterleaveRanker `json:"variations"`
}

// InterleavingDefinitions maps interleaving experiment keys to their
// definitions.
type InterleavingDefinitions map[string]InterleavingDefinition

// UnmarshalJSON decodes leniently, like ContextualBanditDefinitions, so a bad
// section can't block the payload it arrived with: a section that isn't an
// object yields an empty map, and a field of the wrong type is left empty.
func (defs *InterleavingDefinitions) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		*defs = InterleavingDefinitions{}
		return nil
	}
	out := make(InterleavingDefinitions, len(raw))
	for key, rawDef := range raw {
		var def InterleavingDefinition
		_ = json.Unmarshal(rawDef, &def)
		out[key] = def
	}
	*defs = out
	return nil
}

// InterleaveRankers returns the rankers of every interleaving experiment in
// the SDK payload, keyed by experiment key, with the control ranker first.
//
// It is for pre-computing rankings in a background job. Targeting is not
// evaluated: this is every ranker that could be served to someone, not what a
// given user gets. It returns an empty map until a payload has loaded.
//
// An experiment is left out when it doesn't have exactly two rankers, or when
// its "$interleave:" feature isn't in the payload, since it can't be
// interleaved then.
//
// The server sends interleavings only to SDK connections whose SDK version
// supports them.
//
// The result is a copy, so changing it doesn't affect the client.
func (client *Client) InterleaveRankers() map[string][]InterleaveRanker {
	client.data.mu.RLock()
	stored := make(map[string][]InterleaveRanker, len(client.data.interleavings))
	for key := range client.data.interleavings {
		if rankers := client.data.interleaveRankers(key); rankers != nil {
			stored[key] = rankers
		}
	}
	client.data.mu.RUnlock()
	out := make(map[string][]InterleaveRanker, len(stored))
	for key, rankers := range stored {
		out[key] = copyInterleaveRankers(rankers)
	}
	return out
}

// InterleaveRankersForKey returns the rankers of one interleaving experiment,
// as InterleaveRankers does. It returns nil when the experiment isn't in the
// payload.
func (client *Client) InterleaveRankersForKey(key string) []InterleaveRanker {
	client.data.mu.RLock()
	rankers := client.data.interleaveRankers(key)
	client.data.mu.RUnlock()
	return copyInterleaveRankers(rankers)
}

// interleaveRankers returns the client's own rankers and needs d.mu held. A
// payload replaces them rather than changing them, so they can be read after
// the lock is released.
func (d *data) interleaveRankers(key string) []InterleaveRanker {
	def, ok := d.interleavings[key]
	if !ok || len(def.Variations) != 2 {
		return nil
	}
	if _, ok := d.features[interleaveFeaturePrefix+key]; !ok {
		return nil
	}
	return def.Variations
}

func copyInterleaveRankers(rankers []InterleaveRanker) []InterleaveRanker {
	if rankers == nil {
		return nil
	}
	out := make([]InterleaveRanker, len(rankers))
	for i, r := range rankers {
		config, _ := copyJSONValue(r.Config).(map[string]any)
		if config == nil {
			config = map[string]any{}
		}
		out[i] = InterleaveRanker{Key: r.Key, Config: config}
	}
	return out
}

// copyJSONValue deep-copies a decoded JSON value. Other Go containers, which
// only definitions set from Go values hold, are copied through JSON.
func copyJSONValue(v any) any {
	switch v := v.(type) {
	case nil, string, bool, float64, json.Number:
		return v
	case map[string]any:
		if v == nil {
			return v
		}
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = copyJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = copyJSONValue(item)
		}
		return out
	}
	if copied, err := jsonValue(v); err == nil {
		return copied
	}
	return v
}
