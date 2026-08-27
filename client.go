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
	groups               map[string]bool
	qaMode               bool
	experimentCallback   ExperimentCallback
	featureUsageCallback FeatureUsageCallback
	eventLogger          EventLogger
	logger               *slog.Logger
	extraData            any
	// StickyBucketService for storing experiment assignments
	stickyBucketService StickyBucketService

	// StickyBucketAttributes for identifying users
	stickyBucketAttributes StickyBucketAttributes

	// stickyBucketAssignments caches assignments. Shared by reference with
	// cloned clients and mutated during evaluation, hence mutex-guarded.
	stickyBucketAssignments *lockedStickyBucketCache

	// deferredTracks buffers experiment exposures when deferred tracking is
	// enabled. Shared by reference with cloned clients, hence mutex-guarded.
	deferredTracks *trackingBuffer
}

// ForcedVariationsMap is a map that forces an Experiment to always assign a specific variation. Useful for QA.
type ForcedVariationsMap map[string]int

// ExperimentCallback function that is executed every time a user is included
// in an Experiment, with the user context the evaluation ran with.
type ExperimentCallback func(context.Context, *Experiment, *ExperimentResult, *TrackingUserContext, any)

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

	// Seed features from the cache before starting the data source so they are
	// available immediately and survive an initial API failure (offline
	// resilience).
	client.seedFromCache(ctx)

	if client.data.dataSource != nil {
		go client.startDataSource(ctx)
	}

	return client, nil
}

// seedFromCache populates features from the configured FeatureCache when the
// client has no features yet, parsing the stored raw payload through the normal
// update path. It is a no-op when no cache is configured or features are already
// set (so an inline WithFeatures is respected).
func (client *Client) seedFromCache(ctx context.Context) {
	cache := client.data.getFeatureCache()
	if cache == nil || client.data.getFeatures() != nil {
		return
	}
	entry, found, err := cache.Get(ctx, client.data.cacheKey())
	if err != nil {
		client.logger.WarnContext(ctx, "Feature cache read failed on startup, continuing without it", "error", err)
		return
	}
	if !found || entry == nil || len(entry.Payload) == 0 {
		return
	}
	var resp FeatureApiResponse
	if err := json.Unmarshal(entry.Payload, &resp); err != nil {
		client.logger.WarnContext(ctx, "Failed to parse cached features", "error", err)
		return
	}
	resp.raw = entry.Payload
	resp.Etag = entry.Etag
	// Apply without writing back: a seed must not refresh a TTL backend's expiry.
	if err := client.applyApiResponse(&resp); err != nil {
		client.logger.WarnContext(ctx, "Failed to seed features from cache", "error", err)
		return
	}
	// Record the etag we actually seeded with so the poll data source can issue a
	// conditional first request. Only set when the cache was adopted, so inline
	// features (WithFeatures) never reuse an unrelated cached etag.
	client.data.withLock(func(d *data) error {
		d.seededEtag = entry.Etag
		return nil
	})
}

// writeCache persists the raw feature payload to the configured FeatureCache. A
// previously stored etag is preserved when this update carries none (e.g. SSE
// events), so conditional requests still work after a restart. Writes are
// skipped when there is no raw payload to store faithfully.
func (client *Client) writeCache(ctx context.Context, payload json.RawMessage, etag string) {
	cache := client.data.getFeatureCache()
	if cache == nil || len(payload) == 0 {
		return
	}
	key := client.data.cacheKey()
	if etag == "" {
		if prev, found, err := cache.Get(ctx, key); err == nil && found && prev != nil {
			etag = prev.Etag
		}
	}
	if err := cache.Set(ctx, key, &FeatureCacheEntry{Payload: payload, Etag: etag}); err != nil {
		client.logger.WarnContext(ctx, "Failed to write features to cache", "error", err)
	}
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
		stickyBucketAssignments: newStickyBucketCache(defaultStickyBucketCacheSize),
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
	return client.updateFromApiResponse(context.Background(), resp)
}

// updateFromApiResponse applies an API response and writes through to the
// feature cache, using ctx for the cache backend.
func (client *Client) updateFromApiResponse(ctx context.Context, resp *FeatureApiResponse) error {
	if err := client.applyApiResponse(resp); err != nil {
		return err
	}
	client.writeCache(ctx, resp.raw, resp.Etag)
	return nil
}

// applyApiResponse updates the client's shared data from an API response without
// writing through to the feature cache. Seeding uses this so a cache read never
// turns into a cache write (which would refresh a TTL backend's expiry).
func (client *Client) applyApiResponse(resp *FeatureApiResponse) error {
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
	// Preserve the raw payload so it can be persisted verbatim to the cache.
	resp.raw = json.RawMessage(respJSON)
	return client.updateFromApiResponse(context.Background(), &resp)
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
	return client.updateFromApiResponse(ctx, resp)
}

// EvalFeature evaluates feature based on attributes and features map
func (client *Client) EvalFeature(ctx context.Context, key string) *FeatureResult {
	e := client.evaluator(ctx)
	res := e.evalFeature(key)
	client.fireTracking(ctx, e)
	return res
}

func (client *Client) RunExperiment(ctx context.Context, exp *Experiment) *ExperimentResult {
	e := client.evaluator(ctx)
	res := e.runExperiment(exp, "")
	client.fireTracking(ctx, e)
	if client.data.subscribers.hasSubscribers() {
		client.notifySubscribers(ctx, exp, res)
	}
	return res
}

// fireTracking reports one evaluation's tracking data to the deferred
// tracking buffer when enabled, then to the configured callbacks and
// plugins. The buffer is written first so a panicking callback cannot lose
// exposures. Plugin panics are recovered so plugins never interrupt
// evaluation.
func (client *Client) fireTracking(ctx context.Context, e *evaluator) {
	if client.deferredTracks != nil {
		// Detach before buffering: this runs on the evaluating goroutine, so
		// nothing the caller later mutates can reach the buffer.
		client.deferredTracks.add(client.detachTrackingData(e.experiments))
	}
	plugins := client.data.getPlugins()
	for _, u := range e.featureUsage {
		if client.featureUsageCallback != nil {
			client.featureUsageCallback(ctx, u.key, u.result, client.extraData)
		}
		for _, p := range plugins {
			client.safePluginFeatureEvaluated(ctx, p, u.key, u.result)
		}
	}
	for _, d := range e.experiments {
		if client.experimentCallback != nil {
			client.experimentCallback(ctx, d.Experiment, d.Result, d.User, client.extraData)
		}
		for _, p := range plugins {
			client.safePluginExperimentViewed(ctx, p, d.Experiment, d.Result)
		}
	}
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
		recording: client.experimentCallback != nil || client.featureUsageCallback != nil ||
			client.deferredTracks != nil || len(client.data.plugins) > 0,
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
