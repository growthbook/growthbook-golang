package growthbook

import (
	"net/http"
	"net/url"
	"time"
)

// Context contains the options for creating a new GrowthBook
// instance.
type Context struct {
	Enabled          bool
	Attributes       Attributes
	URL              *url.URL
	Features         FeatureMap
	ForcedVariations ForcedVariationsMap
	QAMode           bool
	DevMode          bool
	TrackingCallback ExperimentCallback
	OnFeatureUsage   FeatureUsageCallback
	Groups           map[string]bool
	APIHost          string
	ClientKey        string
	DecryptionKey    string
	Overrides        ExperimentOverrides
	CacheTTL         time.Duration
	HTTPClient       *http.Client
}

// ExperimentCallback is a callback function that is executed every
// time a user is included in an Experiment. It is also the type used
// for subscription functions, which are called whenever
// Experiment.Run is called and the experiment result changes,
// independent of whether a user is inncluded in the experiment or
// not.
type ExperimentCallback func(experiment *Experiment, result *Result)

// FeatureUsageCallback is a callback function that is executed every
// time a feature is evaluated.
type FeatureUsageCallback func(key string, result *FeatureResult)

// NewContext creates a context with default settings: enabled, but
// all other fields empty.
func NewContext() *Context {
	return &Context{
		Enabled:          true,
		Attributes:       Attributes{},
		Features:         FeatureMap{},
		ForcedVariations: ForcedVariationsMap{},
		Groups:           map[string]bool{},
		Overrides:        ExperimentOverrides{},
		CacheTTL:         60 * time.Second,
		HTTPClient:       http.DefaultClient,
	}
}

// WithEnabled sets the enabled flag for a context.
func (ctx *Context) WithEnabled(enabled bool) *Context {
	ctx.Enabled = enabled
	return ctx
}

// WithAttributes sets the attributes for a context.
func (ctx *Context) WithAttributes(attributes Attributes) *Context {
	savedAttributes := Attributes{}
	for k, v := range attributes {
		savedAttributes[k] = fixSliceTypes(v)
	}
	ctx.Attributes = savedAttributes
	return ctx
}

// WithURL sets the URL for a context.
func (ctx *Context) WithURL(url *url.URL) *Context {
	ctx.URL = url
	return ctx
}

// WithFeatures sets the features for a context (as a value of type
// FeatureMap, which is a map from feature names to *Feature values).
func (ctx *Context) WithFeatures(features FeatureMap) *Context {
	if features == nil {
		features = FeatureMap{}
	}
	ctx.Features = features
	return ctx
}

// WithForcedVariations sets the forced variations for a context (as a
// value of type ForcedVariationsMap, which is a map from experiment
// keys to variation indexes).
func (ctx *Context) WithForcedVariations(forcedVariations ForcedVariationsMap) *Context {
	if forcedVariations == nil {
		forcedVariations = ForcedVariationsMap{}
	}
	ctx.ForcedVariations = forcedVariations
	return ctx
}

// ForceVariation sets up a forced variation for a feature.
func (ctx *Context) ForceVariation(key string, variation int) {
	ctx.ForcedVariations[key] = variation
}

// UnforceVariation clears a forced variation for a feature.
func (ctx *Context) UnforceVariation(key string) {
	delete(ctx.ForcedVariations, key)
}

// WithQAMode can be used to enable or disable the QA mode for a
// context.
func (ctx *Context) WithQAMode(qaMode bool) *Context {
	ctx.QAMode = qaMode
	return ctx
}

// WithDevMode can be used to enable or disable the development mode
// for a context.
func (ctx *Context) WithDevMode(devMode bool) *Context {
	ctx.DevMode = devMode
	return ctx
}

// WithTrackingCallback is used to set a tracking callback for a
// context.
func (ctx *Context) WithTrackingCallback(callback ExperimentCallback) *Context {
	ctx.TrackingCallback = callback
	return ctx
}

// WithFeatureUsageCallback is used to set a feature usage callback
// for a context.
func (ctx *Context) WithFeatureUsageCallback(callback FeatureUsageCallback) *Context {
	ctx.OnFeatureUsage = callback
	return ctx
}

// WithGroups sets the groups map of a context.
func (ctx *Context) WithGroups(groups map[string]bool) *Context {
	if groups == nil {
		groups = map[string]bool{}
	}
	ctx.Groups = groups
	return ctx
}

// WithAPIHost sets the API host of a context.
func (ctx *Context) WithAPIHost(host string) *Context {
	ctx.APIHost = host
	return ctx
}

// WithClientKey sets the API client key of a context.
func (ctx *Context) WithClientKey(key string) *Context {
	ctx.ClientKey = key
	return ctx
}

// WithDecryptionKey sets the decryption key of a context.
func (ctx *Context) WithDecryptionKey(key string) *Context {
	ctx.DecryptionKey = key
	return ctx
}

// WithOverrides sets the experiment overrides of a context.
func (ctx *Context) WithOverrides(overrides ExperimentOverrides) *Context {
	if overrides == nil {
		overrides = ExperimentOverrides{}
	}
	ctx.Overrides = overrides
	return ctx
}

// WithCacheTTL sets the TTL for the feature cache.
func (ctx *Context) WithCacheTTL(ttl time.Duration) *Context {
	ctx.CacheTTL = ttl
	return ctx
}

// WithHTTPClient can be used to set the HTTP client for a context.
func (ctx *Context) WithHTTPClient(client *http.Client) *Context {
	ctx.HTTPClient = client
	return ctx
}
