package growthbook

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/growthbook/growthbook-golang/internal/condition"
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

	// trackingBuffer collects experiment exposures when one is attached (see
	// WithTrackingBuffer). Shared by reference with cloned clients — the
	// attacher chose the scope — hence mutex-guarded.
	trackingBuffer *TrackingBuffer
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
	d := client.data
	d.updateMu.Lock()
	defer d.updateMu.Unlock()
	// Apply without writing back: a seed must not refresh a TTL backend's expiry.
	applied, err := client.applyApiResponse(&resp)
	if err != nil {
		client.logger.WarnContext(ctx, "Failed to seed features from cache", "error", err)
		return
	}
	if applied == nil {
		return
	}
	// Start the cache snapshot from the seed, so later partial updates merge
	// into what the cache already holds.
	d.cacheSnapshot = mergeCacheSnapshot(nil, client.rawSections(ctx, resp.raw), *applied)
	d.cacheEtag = entry.Etag
	// Record the etag we actually seeded with so the poll data source can issue a
	// conditional first request. Only set when the cache was adopted, so inline
	// features (WithFeatures) never reuse an unrelated cached etag.
	d.withLock(func(d *data) error {
		d.seededEtag = entry.Etag
		return nil
	})
}

// writeCache merges the sections an accepted response applied into the cache
// snapshot and persists it. The latest known etag is kept when this response
// carries none (e.g. SSE events), so conditional requests still work after a
// restart. The snapshot is written only once it holds features. The caller
// must hold updateMu.
func (client *Client) writeCache(ctx context.Context, resp *FeatureApiResponse, applied appliedSections) {
	cache := client.data.getFeatureCache()
	if cache == nil {
		return
	}
	d := client.data
	if resp.Etag != "" {
		d.cacheEtag = resp.Etag
	}
	d.cacheSnapshot = mergeCacheSnapshot(d.cacheSnapshot, client.rawSections(ctx, resp.raw), applied)
	if !snapshotHasFeatures(d.cacheSnapshot) {
		return
	}
	payload, err := json.Marshal(d.cacheSnapshot)
	if err != nil {
		client.logger.WarnContext(ctx, "Failed to encode features for cache", "error", err)
		return
	}
	entry := &FeatureCacheEntry{Payload: payload, Etag: d.cacheEtag, UpdatedAt: time.Now().UTC()}
	if err := cache.Set(ctx, client.data.cacheKey(), entry); err != nil {
		client.logger.WarnContext(ctx, "Failed to write features to cache", "error", err)
	}
}

// rawSections splits a raw payload into its top-level sections, or returns nil
// when there is none to split (e.g. a response passed to UpdateFromApiResponse).
func (client *Client) rawSections(ctx context.Context, raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		client.logger.WarnContext(ctx, "Failed to split payload for cache", "error", err)
		return nil
	}
	return fields
}

// setDirectly applies sections set through the Set* methods rather than from
// an API payload. They have no raw form to cache, so they are dropped from the
// cache snapshot: the cache never pairs stale copies of them with newer
// sections, and is not written again until a payload supplies features.
func (client *Client) setDirectly(sections appliedSections, update dataUpdate) {
	d := client.data
	d.updateMu.Lock()
	defer d.updateMu.Unlock()
	d.withLock(update)
	d.cacheSnapshot = mergeCacheSnapshot(d.cacheSnapshot, nil, sections)
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
	client.setDirectly(appliedSections{features: true}, func(d *data) error {
		d.features = features
		return nil
	})
	return nil
}

// SetContextualBandits updates the shared contextual bandit definitions.
func (client *Client) SetContextualBandits(bandits ContextualBanditDefinitions) error {
	client.setDirectly(appliedSections{bandits: true}, func(d *data) error {
		d.contextualBandits = bandits
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
// feature cache, using ctx for the cache backend. Updates are serialized with
// their cache writes, so accepted responses reach the cache in order and a
// rejected one never reaches it.
func (client *Client) updateFromApiResponse(ctx context.Context, resp *FeatureApiResponse) error {
	client.data.updateMu.Lock()
	defer client.data.updateMu.Unlock()
	applied, err := client.applyApiResponse(resp)
	if err != nil || applied == nil {
		return err
	}
	client.writeCache(ctx, resp, *applied)
	return nil
}

// applyApiResponse updates the client's shared data from an API response without
// writing through to the feature cache. Seeding uses this so a cache read never
// turns into a cache write (which would refresh a TTL backend's expiry). It
// reports which sections it applied, or nil when it rejected an older response.
// The caller must hold updateMu.
func (client *Client) applyApiResponse(resp *FeatureApiResponse) (*appliedSections, error) {
	dataUpdated := client.data.getDateUpdated()
	apiUpdated := resp.DateUpdated
	if apiUpdated.Before(dataUpdated) {
		client.logger.Warn("Api response is older then current data, refuse to update",
			"dataUpdated", dataUpdated, "apiUdpated", apiUpdated)
		return nil, nil
	}
	// Section-presence semantics (Python setPayload parity): a partial
	// payload — e.g. a bandit-only update — must never wipe sections it did
	// not carry. An absent section preserves the previous data; an explicit
	// empty section clears it. For features and savedGroups a JSON null
	// section decodes to a nil map and counts as absent; contextualBandits
	// additionally treats explicit null as a clear via its custom decoder.
	var features FeatureMap
	var err error
	featuresPresent := false
	if resp.EncryptedFeatures != "" {
		features, err = client.DecryptFeatures(resp.EncryptedFeatures)
		if err != nil {
			return nil, err
		}
		featuresPresent = true
	} else if resp.Features != nil {
		features = resp.Features
		featuresPresent = true
	}
	savedGroups := resp.SavedGroups
	if resp.EncryptedSavedGroups != "" {
		// Like DecryptFeatures, decrypt with the client's key, then parse JSON;
		// this section decodes into SavedGroups instead of FeatureMap.
		groupsJSON, err := client.data.decrypt(resp.EncryptedSavedGroups)
		var decrypted condition.SavedGroups
		if err == nil {
			err = json.Unmarshal([]byte(groupsJSON), &decrypted)
		}
		if err != nil {
			// Match the reference SDK: ignore a failed encrypted section and
			// retain any plaintext fallback or previously loaded groups.
			client.logger.Error("Ignoring undecodable encrypted saved groups; using plaintext fallback or previous groups without blocking feature updates", "error", err)
		} else {
			savedGroups = decrypted
		}
	}
	// Direct callers may supply native Go definitions instead of decoded JSON.
	savedGroups, err = savedGroups.Normalize()
	if err != nil {
		return nil, err
	}
	savedGroupsPresent := savedGroups != nil
	// Section-presence semantics: an absent contextualBandits section
	// preserves the previous definitions, an explicit empty (or null)
	// section clears them, and a section that fails to decrypt is ignored
	// like an absent one — the previous coherent map stays active rather
	// than being wiped by a broken update (Python SDK parity).
	bandits := resp.ContextualBandits
	banditsPresent := bandits != nil
	if resp.EncryptedContextualBandits != "" {
		banditsJSON, err := client.data.decrypt(resp.EncryptedContextualBandits)
		if err == nil {
			err = json.Unmarshal([]byte(banditsJSON), &bandits)
		}
		if err != nil {
			client.logger.Warn("Ignoring undecodable encrypted contextual bandits, keeping the previous definitions", "error", err)
		} else {
			banditsPresent = true
		}
	}
	client.data.withLock(func(d *data) error {
		if featuresPresent {
			d.features = features
		}
		if savedGroupsPresent {
			d.savedGroups = savedGroups
		}
		if banditsPresent {
			d.contextualBandits = bandits
		}
		d.dateUpdated = resp.DateUpdated
		return nil
	})
	return &appliedSections{
		features:    featuresPresent,
		savedGroups: savedGroupsPresent,
		bandits:     banditsPresent,
	}, nil
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
	return client.updateFromApiResponseJSON(context.Background(), respJSON)
}

// updateFromApiResponseJSON parses and applies a JSON API response, using ctx
// for the cache backend so data sources can cancel a stalled write.
func (client *Client) updateFromApiResponseJSON(ctx context.Context, respJSON string) error {
	var resp FeatureApiResponse
	err := json.Unmarshal([]byte(respJSON), &resp)
	if err != nil {
		return err
	}
	// Preserve the raw payload so it can be persisted verbatim to the cache.
	resp.raw = json.RawMessage(respJSON)
	return client.updateFromApiResponse(ctx, &resp)
}

// RefreshFeatures immediately fetches the latest features from the GrowthBook API
// and updates the client. Useful in manual mode when no background datasource is
// configured and the caller wants to control when features are refreshed.
func (client *Client) RefreshFeatures(ctx context.Context) error {
	resp, err := client.CallFeatureApi(ctx, "")
	if err != nil {
		return err
	}
	// Apply partial responses too: UpdateFromApiResponse preserves omitted
	// sections, so a bandit-only or saved-groups-only response updates just
	// what it carries. (An early return on absent features predated that
	// gating and left manual-mode clients with stale bandit definitions.)
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
	// Copy: evaluation may adjust bandit metadata (propensity resync, strip
	// of unused attribution) and must never mutate the caller's experiment.
	expCopy := *exp
	exp = &expCopy
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
	if client.trackingBuffer != nil {
		// Detach before buffering: this runs on the evaluating goroutine, so
		// nothing the caller later mutates can reach the buffer.
		client.trackingBuffer.add(detachTrackingData(e.experiments, client.logger))
	}
	plugins := client.data.getPlugins()
	// Built-in events flow through the event-logger channel alongside the
	// callbacks, mirroring the JS SDK (core.ts onExperimentViewed /
	// onFeatureUsage fire eventLogger per tracked item). Resolve the user
	// context once; exposures carry their own snapshot.
	dispatchEvents := client.hasEventConsumers()
	var usageCtx *EventUserContext
	if dispatchEvents && len(e.featureUsage) > 0 {
		if usageCtx = e.userCtx; usageCtx == nil {
			usageCtx = client.trackingUserContext()
		}
	}
	for _, u := range e.featureUsage {
		if client.featureUsageCallback != nil {
			client.featureUsageCallback(ctx, u.key, u.result, client.extraData)
		}
		for _, p := range plugins {
			client.safePluginFeatureEvaluated(ctx, p, u.key, u.result)
		}
		if dispatchEvents {
			client.dispatchEvent(ctx, EventFeatureEvaluated, featureEvaluatedProperties(u.key, u.result), usageCtx)
		}
	}
	for _, d := range e.experiments {
		if client.experimentCallback != nil {
			client.experimentCallback(ctx, d.Experiment, d.Result, d.User, client.extraData)
		}
		for _, p := range plugins {
			client.safePluginExperimentViewed(ctx, p, d.Experiment, d.Result)
		}
		if dispatchEvents {
			client.dispatchEvent(ctx, EventExperimentViewed, experimentViewedProperties(d.Experiment, d.Result), d.User)
		}
	}
}

// experimentViewedProperties builds the "Experiment Viewed" event properties,
// matching the JS SDK (core.ts onExperimentViewed): variationId is the
// variation key, not the numeric index.
func experimentViewedProperties(exp *Experiment, res *ExperimentResult) EventProperties {
	return EventProperties{
		"experimentId":  exp.Key,
		"variationId":   res.Key,
		"hashAttribute": res.HashAttribute,
		"hashValue":     res.HashValue,
	}
}

// featureEvaluatedProperties builds the "Feature Evaluated" event properties,
// matching the JS SDK (core.ts onFeatureUsage).
func featureEvaluatedProperties(key string, res *FeatureResult) EventProperties {
	ruleId := res.RuleId
	if res.Source == DefaultValueResultSource {
		ruleId = "$default"
	}
	variationId := ""
	if res.ExperimentResult != nil {
		variationId = res.ExperimentResult.Key
	}
	return EventProperties{
		"feature":     key,
		"source":      string(res.Source),
		"value":       res.Value,
		"ruleId":      ruleId,
		"variationId": variationId,
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
		features:          client.data.features,
		savedGroups:       client.data.savedGroups,
		contextualBandits: client.data.contextualBandits,
		client:            client,
		ctx:               ctx,
	}
	// Plugins and the event logger consume both exposures and feature usage;
	// the tracking buffer and ExperimentCallback only exposures, and
	// FeatureUsageCallback only feature usage (see fireTracking).
	both := client.eventLogger != nil || len(client.data.plugins) > 0
	e.recordingExperiments = both || client.experimentCallback != nil || client.trackingBuffer != nil
	e.recordingFeatureUsage = both || client.featureUsageCallback != nil
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
