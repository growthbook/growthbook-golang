package growthbook

import (
	"encoding/json"
	"net/url"
	"regexp"
	"time"
)

// ExperimentOverride provides the possibility to temporarily override
// some experiment settings.
type ExperimentOverride struct {
	Condition Condition
	Weights   []float64
	Active    *bool
	Status    *ExperimentStatus
	Force     *int
	Coverage  *float64
	Groups    []string
	Namespace *Namespace
	URL       *regexp.Regexp
}

type ExperimentOverrides map[string]*ExperimentOverride

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
	UserAttributes   Attributes
	Groups           map[string]bool
	APIHost          string
	ClientKey        string
	DecryptionKey    string
	Overrides        ExperimentOverrides
	CacheTTL         time.Duration
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
		Enabled:  true,
		CacheTTL: 60 * time.Second,
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

// WithUserAttributes sets the user attributes for a context.
func (ctx *Context) WithUserAttributes(attributes Attributes) *Context {
	savedAttributes := Attributes{}
	for k, v := range attributes {
		savedAttributes[k] = fixSliceTypes(v)
	}
	ctx.UserAttributes = savedAttributes
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
	ctx.Features = features
	return ctx
}

// WithForcedVariations sets the forced variations for a context (as a
// value of type ForcedVariationsMap, which is a map from experiment
// keys to variation indexes).
func (ctx *Context) WithForcedVariations(forcedVariations ForcedVariationsMap) *Context {
	ctx.ForcedVariations = forcedVariations
	return ctx
}

func (ctx *Context) ForceVariation(key string, variation int) {
	if ctx.ForcedVariations == nil {
		ctx.ForcedVariations = ForcedVariationsMap{}
	}
	ctx.ForcedVariations[key] = variation
}

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
	ctx.Overrides = overrides
	return ctx
}

// WithCacheTTL sets the TTL for the feature cache.
func (ctx *Context) WithCacheTTL(ttl time.Duration) *Context {
	ctx.CacheTTL = ttl
	return ctx
}

// ParseContext creates a Context value from raw JSON input.
func ParseContext(data []byte) *Context {
	dict := make(map[string]interface{})
	err := json.Unmarshal(data, &dict)
	if err != nil {
		logError("Failed parsing JSON input", "Context")
		return NewContext()
	}
	return BuildContext(dict)
}

// BuildContext creates a Context value from a JSON object represented
// as a Go map.
func BuildContext(dict map[string]interface{}) *Context {
	context := NewContext()
	for k, v := range dict {
		switch k {
		case "enabled":
			enabled, ok := v.(bool)
			if ok {
				context = context.WithEnabled(enabled)
			} else {
				logWarn("Invalid 'enabled' field in JSON context data")
			}
		case "attributes":
			attrs, ok := v.(map[string]interface{})
			if ok {
				context = context.WithAttributes(attrs)
			} else {
				logWarn("Invalid 'attributes' field in JSON context data")
			}
		case "url":
			urlString, ok := v.(string)
			if ok {
				url, err := url.Parse(urlString)
				if err != nil {
					logError("Invalid URL in JSON context data", urlString)
				} else {
					context = context.WithURL(url)
				}
			} else {
				logWarn("Invalid 'url' field in JSON context data")
			}
		case "features":
			features, ok := v.(map[string]interface{})
			if ok {
				context.Features = BuildFeatureMap(features)
			} else {
				logWarn("Invalid 'features' field in JSON context data")
			}
		case "forcedVariations":
			forcedVariations, ok := v.(map[string]interface{})
			if ok {
				vars := make(map[string]int)
				allVOK := true
				for k, vr := range forcedVariations {
					v, vok := vr.(float64)
					if !vok {
						allVOK = false
						break
					}
					vars[k] = int(v)
				}
				if allVOK {
					context = context.WithForcedVariations(vars)
				} else {
					ok = false
				}
			}
			if !ok {
				logWarn("Invalid 'forcedVariations' field in JSON context data")
			}
		case "qaMode":
			qaMode, ok := v.(bool)
			if ok {
				context = context.WithQAMode(qaMode)
			} else {
				logWarn("Invalid 'qaMode' field in JSON context data")
			}
		case "groups":
			groups, ok := v.(map[string]bool)
			if ok {
				context = context.WithGroups(groups)
			} else {
				logWarn("Invalid 'groups' field in JSON context data")
			}
		case "apiHost":
			apiHost, ok := v.(string)
			if ok {
				context = context.WithAPIHost(apiHost)
			} else {
				logWarn("Invalid 'apiHost' field in JSON context data")
			}
		case "clientKey":
			clientKey, ok := v.(string)
			if ok {
				context = context.WithClientKey(clientKey)
			} else {
				logWarn("Invalid 'clientKey' field in JSON context data")
			}
		case "decryptionKey":
			decryptionKey, ok := v.(string)
			if ok {
				context = context.WithDecryptionKey(decryptionKey)
			} else {
				logWarn("Invalid 'decryptionKey' field in JSON context data")
			}
		default:
			logWarn("Unknown key in JSON data", "Context", k)
		}
	}
	return context
}
