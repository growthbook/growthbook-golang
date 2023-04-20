package growthbook

import (
	"encoding/json"
	"net/url"
	"reflect"
)

// Context contains the options for creating a new GrowthBook
// instance.
type Context struct {
	Enabled          bool
	Valid            bool
	Attributes       Attributes
	URL              *url.URL
	Features         FeatureMap
	ForcedVariations ForcedVariationsMap
	QAMode           bool
	TrackingCallback ExperimentCallback
}

// ExperimentCallback is a callback function that is executed every
// time a user is included in an Experiment. It is also the type used
// for subscription functions, which are called whenever
// Experiment.Run is called and the experiment result changes,
// independent of whether a user is inncluded in the experiment or
// not.
type ExperimentCallback func(experiment *Experiment, result *ExperimentResult)

// NewContext creates a context with default settings: enabled, but
// all other fields empty.
func NewContext() *Context {
	return &Context{Enabled: true, Valid: true}
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
		// Skip attributes that are arrays: not allowed.
		if reflect.ValueOf(v).Kind() == reflect.Array {
			ctx.Valid = false
			logError(ErrCtxArrayInAttributes, k)
			continue
		}
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

// WithQAMode can be used to enable or disable the QA mode for a
// context.
func (ctx *Context) WithQAMode(qaMode bool) *Context {
	ctx.QAMode = qaMode
	return ctx
}

// WithTrackingCallback is used to set a tracking callback for a
// context.
func (ctx *Context) WithTrackingCallback(trackingCallback ExperimentCallback) *Context {
	ctx.TrackingCallback = trackingCallback
	return ctx
}

// ParseContext creates a Context value from raw JSON input.
func ParseContext(data []byte) *Context {
	dict := map[string]interface{}{}
	err := json.Unmarshal(data, &dict)
	if err != nil {
		logError(ErrJSONFailedToParse, "Context")
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
			context = context.WithEnabled(v.(bool))
		case "attributes":
			context = context.WithAttributes(v.(map[string]interface{}))
		case "url":
			url, err := url.Parse(v.(string))
			if err != nil {
				logError(ErrCtxJSONInvalidURL, v.(string))
			} else {
				context = context.WithURL(url)
			}
		case "features":
			context.Features = BuildFeatureMap(v.(map[string]interface{}))
		case "forcedVariations":
			vars := map[string]int{}
			for k, vr := range v.(map[string]interface{}) {
				vars[k] = int(vr.(float64))
			}
			context = context.WithForcedVariations(vars)
		case "qaMode":
			context = context.WithQAMode(v.(bool))
		default:
			logWarn(WarnJSONUnknownKey, "Context", k)
		}
	}
	return context
}
