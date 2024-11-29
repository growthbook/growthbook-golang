package growthbook

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/growthbook/growthbook-golang/internal/value"
)

const defaultApiHost = "https://cdn.growthbook.io"

var (
	ErrNoDecryptionKey = errors.New("No decryption key provided")
)

type Client struct {
	data             *data
	enabled          bool
	attributes       value.ObjValue
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

// SetFeatures updates shared client features.
func (client *Client) SetFeatures(features FeatureMap) error {
	client.data.mu.Lock()
	defer client.data.mu.Unlock()

	client.data.features = features
	return nil
}

// SetJSONFeatures updates shared features from JSON
func (client *Client) SetJSONFeatures(featuresJSON string) error {
	var features FeatureMap
	err := json.Unmarshal([]byte(featuresJSON), &features)
	if err != nil {
		return err
	}
	return client.SetFeatures(features)
}

// SetEncryptedJSONFeatures updates shared features from encrypted JSON.
// Uses client's decryption key.
func (client *Client) SetEncryptedJSONFeatures(encryptedJSON string) error {
	if client.data.decryptionKey == "" {
		return ErrNoDecryptionKey
	}
	featuresJSON, err := decrypt(encryptedJSON, client.data.decryptionKey)
	if err != nil {
		return err
	}
	return client.SetJSONFeatures(featuresJSON)
}

// EvalFeature evaluates feature based on attributes and features map
func (client *Client) EvalFeature(ctx context.Context, key string) *FeatureResult {
	client.data.mu.RLock()
	e := &evaluator{
		attributes: client.attributes,
		features:   client.data.features,
	}
	client.data.mu.RUnlock()
	return e.evalFeature(key)
}

// Internals
func (client *Client) clone() *Client {
	c := *client
	return &c
}
