package growthbook

import (
	"net/http"
	"net/url"
)

// Options contains the options for creating a new GrowthBook client
// instance.
type Options struct {
	Disabled            bool
	URL                 *url.URL
	QAMode              bool
	DevMode             bool
	ExperimentTracker   ExperimentTracker
	FeatureUsageTracker FeatureUsageTracker
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
