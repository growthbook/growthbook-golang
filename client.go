package growthbook

import (
	"context"
	"log/slog"
	"net/http"
)

const defaultApiHost = "https://cdn.growthbook.io"

type Client struct {
	data             *data
	enabled          bool
	attributes       Attributes
	url              string
	forcedVariations ForcedVariationsMap
	qaMode           bool
	trackingCallback (TrackingCallback)
	logger           *slog.Logger
}

// ForcedVariationsMap is a map that forces an Experiment to always assign a specific variation. Useful for QA.
type ForcedVariationsMap map[string]int

// TrackingCallback function that is executed every time a user is included in an Experiment.
type TrackingCallback func(*Experiment, *Result)

func NewApiClient(apiHost string, clientKey string) (*Client, error) {
	ctx := context.Background()
	return NewClient(ctx, WithApiHost(apiHost), WithClientKey(clientKey))
}

func NewClient(ctx context.Context, opts ...ClientOption) (*Client, error) {
	client := defaultClient()
	for _, opt := range opts {
		err := opt(client)
		if err != nil {
			return nil, err
		}
	}
	return client, nil
}

func defaultClient() *Client {
	return &Client{
		data: &data{
			apiHost:    defaultApiHost,
			httpClient: http.DefaultClient,
		},
		enabled: true,
		qaMode:  false,
		logger:  slog.Default(),
	}
}

// SetFeatures update shared features in state.
func (client *Client) SetFeatures(features FeatureMap) *Client {
	client.data.mu.Lock()
	defer client.data.mu.Unlock()

	client.data.features = features
	return client
}

// EvalFeature evaluates feature based on attributes and features map
func (client *Client) EvalFeature(ctx context.Context, key string) *FeatureResult {
	client.data.mu.RLock()
	features := client.data.features
	client.data.mu.RUnlock()

	return features.Eval(key)
}

// Internals
func (client *Client) clone() *Client {
	c := *client
	return &c
}
