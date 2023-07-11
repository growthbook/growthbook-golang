package growthbook

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/barkimedes/go-deepcopy"
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

func (o *ExperimentOverride) Copy() *ExperimentOverride {
	retval := ExperimentOverride{}
	if o.Condition != nil {
		retval.Condition = deepcopy.MustAnything(o.Condition).(Condition)
	}
	if o.Weights != nil {
		retval.Weights = make([]float64, len(o.Weights))
		copy(retval.Weights, o.Weights)
	}
	if o.Active != nil {
		tmp := *o.Active
		retval.Active = &tmp
	}
	if o.Status != nil {
		tmp := *o.Status
		retval.Status = &tmp
	}
	if o.Force != nil {
		tmp := *o.Force
		retval.Force = &tmp
	}
	if o.Coverage != nil {
		tmp := *o.Coverage
		retval.Coverage = &tmp
	}
	if o.Groups != nil {
		retval.Groups = make([]string, len(o.Groups))
		copy(retval.Groups, o.Groups)
	}
	if o.Namespace != nil {
		retval.Namespace = o.Namespace.Copy()
	}
	if o.URL != nil {
		tmp := regexp.Regexp(*o.URL)
		retval.URL = &tmp
	}
	return &retval
}

type ExperimentOverrides map[string]*ExperimentOverride

func (os ExperimentOverrides) Copy() ExperimentOverrides {
	retval := map[string]*ExperimentOverride{}
	for k, v := range os {
		retval[k] = v.Copy()
	}
	return retval
}

// Context contains the options for creating a new GrowthBook
// instance.
type Context struct {
	enabled          bool
	attributes       Attributes
	url              *url.URL
	features         FeatureMap
	forcedVariations ForcedVariationsMap
	qaMode           bool
	devMode          bool
	trackingCallback ExperimentCallback
	onFeatureUsage   FeatureUsageCallback
	groups           map[string]bool
	apiHost          string
	clientKey        string
	decryptionKey    string
	overrides        ExperimentOverrides
	cacheTTL         time.Duration
	httpClient       *http.Client
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
		enabled:          true,
		attributes:       Attributes{},
		features:         FeatureMap{},
		forcedVariations: ForcedVariationsMap{},
		groups:           map[string]bool{},
		overrides:        ExperimentOverrides{},
		cacheTTL:         60 * time.Second,
		httpClient:       http.DefaultClient,
	}
}

// Enabled returns the current enabled flag.
func (ctx *Context) Enabled() bool { return ctx.enabled }

// WithEnabled sets the enabled flag for a context.
func (ctx *Context) WithEnabled(enabled bool) *Context {
	ctx.enabled = enabled
	return ctx
}

// Attributes returns the current attributes for a context.
func (ctx *Context) Attributes() Attributes {
	return ctx.attributes
}

// WithAttributes sets the attributes for a context.
func (ctx *Context) WithAttributes(attributes Attributes) *Context {
	savedAttributes := Attributes{}
	for k, v := range attributes {
		savedAttributes[k] = fixSliceTypes(v)
	}
	ctx.attributes = savedAttributes
	return ctx
}

// URL returns the URL for a context.
func (ctx *Context) URL() *url.URL {
	return ctx.url
}

// WithURL sets the URL for a context.
func (ctx *Context) WithURL(url *url.URL) *Context {
	ctx.url = url
	return ctx
}

// Features returns the current features for a context (as a value of
// type FeatureMap, which is a map from feature names to *Feature
// values).
func (ctx *Context) Features() FeatureMap {
	return ctx.features
}

// WithFeatures sets the features for a context (as a value of type
// FeatureMap, which is a map from feature names to *Feature values).
func (ctx *Context) WithFeatures(features FeatureMap) *Context {
	if features == nil {
		features = FeatureMap{}
	}
	ctx.features = features
	return ctx
}

// ForcedVariations returns the forced variations for a context (as a
// value of type ForcedVariationsMap, which is a map from experiment
// keys to variation indexes).
func (ctx *Context) ForcedVariations() ForcedVariationsMap {
	return ctx.forcedVariations
}

// WithForcedVariations sets the forced variations for a context (as a
// value of type ForcedVariationsMap, which is a map from experiment
// keys to variation indexes).
func (ctx *Context) WithForcedVariations(forcedVariations ForcedVariationsMap) *Context {
	if forcedVariations == nil {
		forcedVariations = ForcedVariationsMap{}
	}
	ctx.forcedVariations = forcedVariations
	return ctx
}

// ForceVariation sets up a forced variation for a feature.
func (ctx *Context) ForceVariation(key string, variation int) {
	ctx.forcedVariations[key] = variation
}

// UnforceVariation clears a forced variation for a feature.
func (ctx *Context) UnforceVariation(key string) {
	delete(ctx.forcedVariations, key)
}

// QAMode returns the current QA mode setting for a context.
func (ctx *Context) QAMode() bool {
	return ctx.qaMode
}

// WithQAMode can be used to enable or disable the QA mode for a
// context.
func (ctx *Context) WithQAMode(qaMode bool) *Context {
	ctx.qaMode = qaMode
	return ctx
}

// DevMode returns the development mode setting for a context.
func (ctx *Context) DevMode() bool {
	return ctx.devMode
}

// WithDevMode can be used to enable or disable the development mode
// for a context.
func (ctx *Context) WithDevMode(devMode bool) *Context {
	ctx.devMode = devMode
	return ctx
}

// TrackingCallback return the current tracking callback for a
// context.
func (ctx *Context) TrackingCallback() ExperimentCallback {
	return ctx.trackingCallback
}

// WithTrackingCallback is used to set a tracking callback for a
// context.
func (ctx *Context) WithTrackingCallback(callback ExperimentCallback) *Context {
	ctx.trackingCallback = callback
	return ctx
}

// FeatureUsageCallback returns the current feature usage callback for
// a context.
func (ctx *Context) FeatureUsageCallback() FeatureUsageCallback {
	return ctx.onFeatureUsage
}

// WithFeatureUsageCallback is used to set a feature usage callback
// for a context.
func (ctx *Context) WithFeatureUsageCallback(callback FeatureUsageCallback) *Context {
	ctx.onFeatureUsage = callback
	return ctx
}

// Groups returns the groups map of a context.
func (ctx *Context) Groups() map[string]bool {
	return ctx.groups
}

// WithGroups sets the groups map of a context.
func (ctx *Context) WithGroups(groups map[string]bool) *Context {
	if groups == nil {
		groups = map[string]bool{}
	}
	ctx.groups = groups
	return ctx
}

// APIHost returns the API host of a context.
func (ctx *Context) APIHost() string {
	return ctx.apiHost
}

// WithAPIHost sets the API host of a context.
func (ctx *Context) WithAPIHost(host string) *Context {
	ctx.apiHost = host
	return ctx
}

// WithClientKey sets the API client key of a context.
func (ctx *Context) WithClientKey(key string) *Context {
	ctx.clientKey = key
	return ctx
}

// ClientKey returns the API client key of a context.
func (ctx *Context) ClientKey() string {
	return ctx.clientKey
}

// DecryptionKey returns the decryption key of a context.
func (ctx *Context) DecryptionKey() string {
	return ctx.decryptionKey
}

// WithDecryptionKey sets the decryption key of a context.
func (ctx *Context) WithDecryptionKey(key string) *Context {
	ctx.decryptionKey = key
	return ctx
}

// Overrides returns the experiment overrides of a context.
func (ctx *Context) Overrides() ExperimentOverrides {
	return ctx.overrides
}

// WithOverrides sets the experiment overrides of a context.
func (ctx *Context) WithOverrides(overrides ExperimentOverrides) *Context {
	if overrides == nil {
		overrides = ExperimentOverrides{}
	}
	ctx.overrides = overrides
	return ctx
}

// CacheTTL returns the TTL for the feature cache.
func (ctx *Context) CacheTTL() time.Duration {
	return ctx.cacheTTL
}

// WithCacheTTL sets the TTL for the feature cache.
func (ctx *Context) WithCacheTTL(ttl time.Duration) *Context {
	ctx.cacheTTL = ttl
	return ctx
}

// HTTPClient returns the HTTP client setting for a context.
func (ctx *Context) HTTPClient() *http.Client {
	return ctx.httpClient
}

// WithHTTPClient can be used to set the HTTP client for a context.
func (ctx *Context) WithHTTPClient(client *http.Client) *Context {
	ctx.httpClient = client
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
				context.features = BuildFeatureMap(features)
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
