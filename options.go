package growthbook

import (
	"net/http"
	"net/url"
	"time"
)

// Options contains the options for creating a new GrowthBook client
// instance.
type Options struct {
	Disabled         bool
	URL              *url.URL
	QAMode           bool
	DevMode          bool
	TrackingCallback ExperimentCallback
	OnFeatureUsage   FeatureUsageCallback
	Groups           map[string]bool
	APIHost          string
	ClientKey        string
	DecryptionKey    string
	CacheTTL         time.Duration
	HTTPClient       *http.Client
}

// ExperimentCallback is a callback function that is executed every
// time a user is included in an Experiment. It is also the type used
// for subscription functions, which are called whenever
// Experiment.Run is called and the experiment result changes,
// independent of whether a user is inncluded in the experiment or
// not.
// TODO: type ExperimentCallback func(ctx context.Context, experiment *Experiment, result *Result)
type ExperimentCallback func(experiment *Experiment, result *Result)

// FeatureUsageCallback is a callback function that is executed every
// time a feature is evaluated.
// TODO: type FeatureUsageCallback func(ctx context.Context, key string, result *FeatureResult)
type FeatureUsageCallback func(key string, result *FeatureResult)

func (opt *Options) defaults() {
	if opt.Groups == nil {
		opt.Groups = map[string]bool{}
	}
	if opt.APIHost == "" {
		opt.APIHost = "https://cdn.growthbook.io"
	}
	if opt.CacheTTL == 0 {
		opt.CacheTTL = 60 * time.Second
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
