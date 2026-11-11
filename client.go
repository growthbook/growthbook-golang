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

	if client.data.dataSource != nil {
		ctx, cancel := context.WithCancel(ctx)
		client.data.dsCancel = cancel
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
	// Also stops a data source that is still on its first load, which
	// ds.Close can't reach yet.
	if client.data.dsCancel != nil {
		client.data.dsCancel()
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
		// Go values carry no payload JSON to reduce
		d.payloadJSON = nil
		return nil
	})
	return nil
}

// SetContextualBandits updates the shared contextual bandit definitions.
func (client *Client) SetContextualBandits(bandits ContextualBanditDefinitions) error {
	client.data.withLock(func(d *data) error {
		d.contextualBandits = bandits
		d.banditsFromCode = true
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
	var raw any
	if err := json.Unmarshal([]byte(featuresJSON), &raw); err != nil {
		return err
	}
	if raw == nil {
		// null clears the features, as it does for evaluation
		raw = map[string]any{}
	}
	client.data.withLock(func(d *data) error {
		d.features = features
		d.payloadJSON = mergePayloadJSON(d.payloadJSON, map[string]any{"features": raw})
		return nil
	})
	return nil
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

// UpdateFromApiResponse updates shared data from Growthbook API response.
//
// Sections follow presence semantics (Python setPayload parity): an absent
// section keeps the previous data and an explicit empty one clears it, so a
// partial payload, e.g. a bandit-only update, never wipes what it didn't
// carry. A response older than the current data is ignored.
func (client *Client) UpdateFromApiResponse(resp *FeatureApiResponse) error {
	_, _, err := client.applyApiResponse(resp)
	return err
}

// applyApiResponse applies an API response to the shared client data. It
// reports whether anything was applied and the dateUpdated the client holds
// afterwards: the response's own timestamp when it was applied, and the
// currently stored one when it was not, so a refresh handler never publishes a
// timestamp the client did not adopt.
func (client *Client) applyApiResponse(resp *FeatureApiResponse) (bool, time.Time, error) {
	dataUpdated := client.data.getDateUpdated()
	if client.isStaleResponse(resp, dataUpdated) {
		return false, dataUpdated, nil
	}
	update, err := client.decodePayloadUpdate(resp)
	if err != nil {
		return false, dataUpdated, err
	}
	// A payload that carries no section at all - or only a section that could
	// not be decoded - applies nothing. It is still a valid no-op response
	// (section-presence semantics), so it is not an error, but it must not be
	// reported as an update: a malformed SSE event that omits "features" would
	// otherwise look like a successful refresh to the refresh handler.
	applied := update.featuresPresent || update.savedGroupsPresent || update.banditsPresent
	if !applied {
		client.logger.Warn("Api response contains nothing to apply, keeping current data",
			"dateUpdated", resp.DateUpdated)
	}
	stale := false
	client.data.withLock(func(d *data) error {
		// Checked again under the lock: a concurrent update may have applied
		// newer data while this one was decoding.
		if client.isStaleResponse(resp, d.dateUpdated) {
			stale, dataUpdated = true, d.dateUpdated
			return nil
		}
		update.apply(d)
		return nil
	})
	if stale {
		return false, dataUpdated, nil
	}
	return applied, resp.DateUpdated, nil
}

func (client *Client) applyApiResponseJSON(respJSON string) (bool, time.Time, error) {
	var resp FeatureApiResponse
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		return false, client.data.getDateUpdated(), err
	}
	return client.applyApiResponse(&resp)
}

func (client *Client) isStaleResponse(resp *FeatureApiResponse, dataUpdated time.Time) bool {
	if !resp.DateUpdated.Before(dataUpdated) {
		return false
	}
	client.logger.Warn("Api response is older then current data, refuse to update",
		"dataUpdated", dataUpdated, "apiUdpated", resp.DateUpdated)
	return true
}

// payloadUpdate is a decoded response. Sections whose present flag is false
// keep the previous data.
type payloadUpdate struct {
	features           FeatureMap
	featuresPresent    bool
	savedGroups        condition.SavedGroups
	savedGroupsPresent bool
	bandits            ContextualBanditDefinitions
	banditsPresent     bool
	// payloadJSON is nil for a response built from Go values
	payloadJSON map[string]any
	dateUpdated time.Time
}

func (client *Client) decodePayloadUpdate(resp *FeatureApiResponse) (*payloadUpdate, error) {
	u := payloadUpdate{dateUpdated: resp.DateUpdated}
	var err error
	if u.features, u.featuresPresent, err = client.decodeFeaturesSection(resp); err != nil {
		return nil, err
	}
	if u.savedGroups, err = client.decodeSavedGroupsSection(resp); err != nil {
		return nil, err
	}
	// A JSON null section decodes to a nil map and counts as absent
	u.savedGroupsPresent = u.savedGroups != nil
	u.bandits, u.banditsPresent = client.decodeBanditsSection(resp)
	if u.payloadJSON, err = client.reducibleUpdateJSON(resp, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (client *Client) decodeFeaturesSection(resp *FeatureApiResponse) (FeatureMap, bool, error) {
	if resp.EncryptedFeatures != "" {
		features, err := client.DecryptFeatures(resp.EncryptedFeatures)
		return features, err == nil, err
	}
	return resp.Features, resp.Features != nil, nil
}

// decodeSavedGroupsSection prefers the encrypted section. One that fails to
// decrypt is ignored, like the reference SDK, leaving the plaintext fallback
// or the previous groups.
func (client *Client) decodeSavedGroupsSection(resp *FeatureApiResponse) (condition.SavedGroups, error) {
	savedGroups := resp.SavedGroups
	if resp.EncryptedSavedGroups != "" {
		var decrypted condition.SavedGroups
		groupsJSON, err := client.data.decrypt(resp.EncryptedSavedGroups)
		if err == nil {
			err = json.Unmarshal([]byte(groupsJSON), &decrypted)
		}
		if err != nil {
			client.logger.Error("Ignoring undecodable encrypted saved groups; using plaintext fallback or previous groups without blocking feature updates", "error", err)
		} else {
			savedGroups = decrypted
		}
	}
	// Direct callers may supply native Go definitions instead of decoded JSON.
	return savedGroups.Normalize()
}

// decodeBanditsSection treats an explicit null as a clear, via the section's
// decoder. An encrypted section that fails to decrypt is ignored like an
// absent one, so the previous definitions stay (Python SDK parity).
func (client *Client) decodeBanditsSection(resp *FeatureApiResponse) (ContextualBanditDefinitions, bool) {
	bandits := resp.ContextualBandits
	present := bandits != nil
	if resp.EncryptedContextualBandits == "" {
		return bandits, present
	}
	banditsJSON, err := client.data.decrypt(resp.EncryptedContextualBandits)
	if err == nil {
		err = json.Unmarshal([]byte(banditsJSON), &bandits)
	}
	if err != nil {
		client.logger.Warn("Ignoring undecodable encrypted contextual bandits, keeping the previous definitions", "error", err)
		return bandits, present
	}
	return bandits, true
}

// reducibleUpdateJSON returns the response's JSON with each section matching
// the decoded update, so ReducePayload never keeps data evaluation dropped,
// or drops data it uses.
func (client *Client) reducibleUpdateJSON(resp *FeatureApiResponse, u *payloadUpdate) (map[string]any, error) {
	payloadJSON := client.decryptedPayloadJSON(resp)
	if payloadJSON == nil {
		return nil, nil
	}
	if !u.featuresPresent {
		delete(payloadJSON, "features")
	}
	delete(payloadJSON, "savedGroups")
	if u.savedGroupsPresent {
		groupsJSON, err := u.savedGroups.JSONValue()
		if err != nil {
			return nil, err
		}
		payloadJSON["savedGroups"] = groupsJSON
	}
	if !u.banditsPresent {
		delete(payloadJSON, "contextualBandits")
	} else if _, ok := payloadJSON["contextualBandits"].(map[string]any); !ok {
		// An explicit null, or junk the decoder reads as none, clears them
		payloadJSON["contextualBandits"] = map[string]any{}
	}
	return payloadJSON, nil
}

func (u *payloadUpdate) apply(d *data) {
	if u.payloadJSON != nil {
		d.payloadJSON = mergePayloadJSON(d.payloadJSON, u.payloadJSON)
	} else {
		d.payloadJSON = nil
	}
	if u.featuresPresent {
		d.features = u.features
	}
	if u.savedGroupsPresent {
		d.savedGroups = u.savedGroups
		d.savedGroupsFromCode = false
	}
	if u.banditsPresent {
		d.contextualBandits = u.bandits
		d.banditsFromCode = false
	}
	d.dateUpdated = u.dateUpdated
}

// decryptedPayloadJSON returns the response's JSON with its encrypted sections
// decrypted, or nil when the response wasn't decoded from JSON. A section that
// fails to decrypt is dropped, like an absent one.
func (client *Client) decryptedPayloadJSON(resp *FeatureApiResponse) map[string]any {
	if resp.raw == nil {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.raw, &payload); err != nil || payload == nil {
		return nil
	}
	for section, encryptedKey := range map[string]string{
		"features":          "encryptedFeatures",
		"savedGroups":       "encryptedSavedGroups",
		"contextualBandits": "encryptedContextualBandits",
		"experiments":       "encryptedExperiments",
	} {
		encrypted, _ := payload[encryptedKey].(string)
		delete(payload, encryptedKey)
		if encrypted == "" {
			continue
		}
		var decrypted any
		plain, err := client.data.decrypt(encrypted)
		if err == nil {
			err = json.Unmarshal([]byte(plain), &decrypted)
		}
		if err == nil {
			payload[section] = decrypted
		}
	}
	return payload
}

// mergePayloadJSON applies an update with section-presence semantics, as
// UpdateFromApiResponse does: a section the update doesn't carry keeps its
// previous value. Callers remove the sections their update doesn't apply.
// Auto experiments, which this SDK doesn't evaluate, also keep the previous
// value on null.
func mergePayloadJSON(prev, update map[string]any) map[string]any {
	merged := make(map[string]any, len(update))
	for k, v := range update {
		merged[k] = v
	}
	for _, section := range []string{"features", "savedGroups", "contextualBandits", "experiments"} {
		v, ok := merged[section]
		if (!ok || (section == "experiments" && v == nil)) && prev[section] != nil {
			merged[section] = prev[section]
		}
	}
	return merged
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
	_, _, err := client.applyApiResponseJSON(respJSON)
	return err
}

// RefreshFeatures immediately fetches the latest features from the GrowthBook API
// and updates the client. Useful in manual mode when no background datasource is
// configured and the caller wants to control when features are refreshed.
// It is RefreshFeaturesWithMode(ctx, RefreshDefault).
func (client *Client) RefreshFeatures(ctx context.Context) error {
	return client.RefreshFeaturesWithMode(ctx, RefreshDefault)
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
