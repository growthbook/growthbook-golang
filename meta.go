package growthbook

// VariationMeta represents meta-information that can be passed
// through to tracking callbacks.
type VariationMeta struct {
	Passthrough bool   `json:"passthrough,omitempty"`
	Key         string `json:"key,omitempty"`
	Name        string `json:"name,omitempty"`
}
