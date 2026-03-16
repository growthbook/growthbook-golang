package growthbook

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/growthbook/growthbook-golang/internal/value"
)

const defaultApiHost = "https://cdn.growthbook.io"

var (
	ErrNoDecryptionKey = errors.New("no decryption key provided")
)

// Client is a GrowthBook SDK client.
type Client struct {
	data                 *data
	enabled              bool
	attributes           value.ObjValue
	url                  *url.URL
	forcedVariations     ForcedVariationsMap
	qaMode               bool
	experimentCallback   ExperimentCallback
	featureUsageCallback FeatureUsageCallback
	logger               *slog.Logger
	extraData            any
	// StickyBucketService for storing experiment assignments
	stickyBucketService StickyBucketService

	// StickyBucketAttributes for identifying users
	stickyBucketAttributes StickyBucketAttributes

	// StickyBucketAssignments caches assignments
	stickyBucketAssignments StickyBucketAssignments
}

// ForcedVariationsMap is a map that forces an Experiment to always assign a specific variation. Useful for QA.
type ForcedVariationsMap map[string]int

// ExperimentCallback function that is executed every time a user is included in an Experiment.
type ExperimentCallback func(context.Context, *Experiment, *ExperimentResult, any)

// FeatureUsageCallback funcion is executed every time feature is evaluated
type FeatureUsageCallback func(context.Context, string, *FeatureResult, any)

// NewApiClient creates simple client with API host and client key
func NewApiClient(apiHost string, clientKey string) (*Client, error) {
	ctx := context.Background()
	return NewClient(ctx, WithApiHost(apiHost), WithClientKey(clientKey))
}

// NewClient create a new GrowthBook SDK client.
func NewClient(ctx context.Context, opts ...ClientOption) (*Client, error) {
	client := defaultClient()
	for _, opt := range opts {
		err := opt(client)
		if err != nil {
			return nil, err
		}
	}

	// Initialize plugins. Errors are logged but do not prevent client
	// creation — plugin functionality must never interfere with SDK
	// evaluation. Plugins that fail Init are kept in the list but must
	// guard their tracking methods against being called uninitialised.
	for _, p := range client.data.plugins {
		if err := p.Init(client); err != nil {
			client.logger.Error("Plugin initialization failed", "error", err)
		}
	}

	if client.data.dataSource != nil {
		go client.startDataSource(ctx)
	}

	return client, nil
}

// Close client's background goroutines and plugins.
func (client *Client) Close() error {
	var errs []error

	// Close plugins first so they can flush remaining events.
	for _, p := range client.data.getPlugins() {
		if err := p.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	ds := client.data.dataSource
	if ds != nil && client.data.getDsStarted() {
		if err := ds.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func defaultClient() *Client {
	return &Client{
		data:                    newData(),
		enabled:                 true,
		qaMode:                  false,
		logger:                  slog.Default(),
		attributes:              value.ObjValue{},
		stickyBucketAssignments: make(StickyBucketAssignments),
	}
}

// SetFeatures updates shared client features.
func (client *Client) SetFeatures(features FeatureMap) error {
	client.data.withLock(func(d *data) error {
		d.features = features
		return nil
	})
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

// UpdateFromApiResponse updates shared data from Growthbook API response
func (client *Client) UpdateFromApiResponse(resp *FeatureApiResponse) error {
	dataUpdated := client.data.getDateUpdated()
	apiUpdated := resp.DateUpdated
	if apiUpdated.Before(dataUpdated) {
		client.logger.Warn("Api response is older then current data, refuse to update",
			"dataUpdated", dataUpdated, "apiUdpated", apiUpdated)
		return nil
	}
	var features FeatureMap
	var err error
	if resp.EncryptedFeatures != "" {
		features, err = client.DecryptFeatures(resp.EncryptedFeatures)
		if err != nil {
			return err
		}
	} else {
		features = resp.Features
	}
	client.data.withLock(func(d *data) error {
		d.features = features
		d.savedGroups = resp.SavedGroups
		d.dateUpdated = resp.DateUpdated
		return nil
	})
	return nil
}

func (client *Client) DecryptFeatures(encrypted string) (FeatureMap, error) {
	var features FeatureMap
	featuresJSON, err := client.data.decrypt(encrypted)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(featuresJSON), &features)
	if err != nil {
		return nil, err
	}
	return features, err
}

func (client *Client) UpdateFromApiResponseJSON(respJSON string) error {
	var resp FeatureApiResponse
	err := json.Unmarshal([]byte(respJSON), &resp)
	if err != nil {
		return err
	}
	return client.UpdateFromApiResponse(&resp)
}

// RefreshFeatures immediately fetches the latest features from the GrowthBook API
// and updates the client. Useful in manual mode when no background datasource is
// configured and the caller wants to control when features are refreshed.
func (client *Client) RefreshFeatures(ctx context.Context) error {
	resp, err := client.CallFeatureApi(ctx, "")
	if err != nil {
		return err
	}
	if resp.Features == nil && resp.EncryptedFeatures == "" {
		return nil
	}
	return client.UpdateFromApiResponse(resp)
}

// EvalFeature evaluates feature based on attributes and features map
func (client *Client) EvalFeature(ctx context.Context, key string) *FeatureResult {
	e := client.evaluator(ctx)
	res := e.evalFeature(key)
	if client.featureUsageCallback != nil {
		client.featureUsageCallback(ctx, key, res, client.extraData)
	}
	if client.experimentCallback != nil && res.InExperiment() {
		client.experimentCallback(ctx, res.Experiment, res.ExperimentResult, client.extraData)
	}
	// Notify plugins. Panics are recovered so plugins never interrupt evaluation.
	for _, p := range client.data.getPlugins() {
		client.safePluginFeatureEvaluated(ctx, p, key, res)
		if res.InExperiment() {
			client.safePluginExperimentViewed(ctx, p, res.Experiment, res.ExperimentResult)
		}
	}
	return res
}

func (client *Client) RunExperiment(ctx context.Context, exp *Experiment) *ExperimentResult {
	e := client.evaluator(ctx)
	res := e.runExperiment(exp, "")
	if client.experimentCallback != nil && res.InExperiment {
		client.experimentCallback(ctx, exp, res, client.extraData)
	}
	// Notify plugins.
	for _, p := range client.data.getPlugins() {
		if res.InExperiment {
			client.safePluginExperimentViewed(ctx, p, exp, res)
		}
	}
	return res
}

func (client *Client) Features() FeatureMap {
	return client.data.getFeatures()
}

// ClientKey returns the SDK client key used to authenticate with the GrowthBook API.
func (client *Client) ClientKey() string {
	return client.data.getClientKey()
}

// HttpClient returns the HTTP client used by the GrowthBook client.
func (client *Client) HttpClient() *http.Client {
	client.data.mu.RLock()
	defer client.data.mu.RUnlock()
	return client.data.httpClient
}

// Logger returns the logger used by the GrowthBook client.
func (client *Client) Logger() *slog.Logger {
	return client.logger
}

// Internals
func (client *Client) evaluator(ctx context.Context) *evaluator {
	client.data.mu.RLock()
	e := evaluator{
		features:    client.data.features,
		savedGroups: client.data.savedGroups,
		client:      client,
		ctx:         ctx,
	}
	client.data.mu.RUnlock()
	return &e
}

func (client *Client) clone() *Client {
	c := *client
	return &c
}

// safePluginExperimentViewed calls the plugin's OnExperimentViewed,
// recovering from any panic so that plugin errors never interrupt SDK functions.
func (client *Client) safePluginExperimentViewed(ctx context.Context, p Plugin, exp *Experiment, res *ExperimentResult) {
	defer func() {
		if r := recover(); r != nil {
			client.logger.ErrorContext(ctx, "Plugin panicked in OnExperimentViewed", "error", r)
		}
	}()
	p.OnExperimentViewed(ctx, exp, res)
}

// safePluginFeatureEvaluated calls the plugin's OnFeatureEvaluated,
// recovering from any panic so that plugin errors never interrupt SDK functions.
func (client *Client) safePluginFeatureEvaluated(ctx context.Context, p Plugin, key string, res *FeatureResult) {
	defer func() {
		if r := recover(); r != nil {
			client.logger.ErrorContext(ctx, "Plugin panicked in OnFeatureEvaluated", "error", r)
		}
	}()
	p.OnFeatureEvaluated(ctx, key, res)
}
