package growthbook

import (
	"context"
	"net/http"
	"net/url"
)

// ExperimentTrackerIf is an interface with a callback method that is
// executed every time a user is included in an Experiment. It is also
// the type used for subscription functions, which are called whenever
// Experiment.Run is called and the experiment result changes,
// independent of whether a user is inncluded in the experiment or
// not.

type ExperimentTrackerIf interface {
	Track(ctx context.Context, c *Client,
		exp *Experiment, result *Result, extraData interface{})
}

// ExperimentCallback is a wrapper around a simple callback for
// experiment tracking.

type ExperimentCallback struct {
	CB func(ctx context.Context, exp *Experiment, result *Result)
}

func (tcb *ExperimentCallback) Track(ctx context.Context,
	c *Client, exp *Experiment, result *Result, extraData interface{}) {
	tcb.CB(ctx, exp, result)
}

// FeatureUsageTrackerIf is an interface with a callback method that
// is executed every time a feature is evaluated.

type FeatureUsageTrackerIf interface {
	OnFeatureUsage(ctx context.Context, c *Client,
		key string, result *FeatureResult, extraData interface{})
}

// FeatureUsageCallback is a wrapper around a simple callback for
// feature usage tracking.

type FeatureUsageCallback struct {
	CB func(ctx context.Context, key string, result *FeatureResult)
}

func (fcb *FeatureUsageCallback) OnFeatureUsage(ctx context.Context,
	c *Client, key string, result *FeatureResult, extraData interface{}) {
	fcb.CB(ctx, key, result)
}

// Options contains the options for creating a new GrowthBook client
// instance.
type Options struct {
	Disabled            bool
	URL                 *url.URL
	QAMode              bool
	DevMode             bool
	ExperimentTracker   ExperimentTrackerIf
	FeatureUsageTracker FeatureUsageTrackerIf
	Groups              map[string]bool
	APIHost             string
	ClientKey           string
	DecryptionKey       string
	HTTPClient          *http.Client
}

func (opt *Options) defaults() {
	if opt.Groups == nil {
		opt.Groups = map[string]bool{}
	}
	if opt.APIHost == "" {
		opt.APIHost = "https://cdn.growthbook.io"
	}
	if opt.HTTPClient == nil {
		opt.HTTPClient = http.DefaultClient
	}
}

func (opt *Options) clone() *Options {
	clone := *opt
	if opt.URL != nil {
		newURL := *opt.URL
		clone.URL = &newURL
	}
	return &clone
}
