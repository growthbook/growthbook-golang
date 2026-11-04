package growthbook

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	"github.com/growthbook/growthbook-golang/internal/value"
)

const (
	defaultIngestorHost = "https://us1.gb-ingest.com"
	defaultBatchSize    = 100
	defaultBatchTimeout = 10 * time.Second
	// defaultDedupeCacheSize matches the JS tracking plugin's dedupeCacheSize.
	defaultDedupeCacheSize = 1000
)

var (
	sdkVersionOnce  sync.Once
	sdkVersionValue string
)

// sdkVersion returns the module version of the growthbook-golang SDK,
// derived from go.mod via runtime/debug.ReadBuildInfo. The result is
// cached after the first call.
func sdkVersion() string {
	sdkVersionOnce.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			sdkVersionValue = "unknown"
			return
		}
		// When used as a dependency, the main module version is the
		// consumer's version. Walk the dependency list instead.
		for _, dep := range info.Deps {
			if dep.Path == "github.com/growthbook/growthbook-golang" {
				sdkVersionValue = dep.Version
				return
			}
		}
		// If this IS the main module (running tests, etc.), use the
		// main module version.
		if info.Main.Path == "github.com/growthbook/growthbook-golang" {
			sdkVersionValue = info.Main.Version
			if sdkVersionValue == "(devel)" || sdkVersionValue == "" {
				sdkVersionValue = "dev"
			}
			return
		}
		sdkVersionValue = "unknown"
	})
	return sdkVersionValue
}

// TrackingPluginConfig configures the GrowthBookTrackingPlugin.
type TrackingPluginConfig struct {
	// IngestorHost is the GrowthBook event ingestor endpoint.
	// Defaults to "https://us1.gb-ingest.com".
	IngestorHost string

	// BatchSize is the maximum number of events to accumulate before
	// flushing. Defaults to 100.
	BatchSize int

	// BatchTimeout is the maximum time to wait before flushing
	// accumulated events. Defaults to 10 seconds.
	BatchTimeout time.Duration

	// HTTPClient is used for sending events. If nil, the client's
	// HTTP client is used (which defaults to http.DefaultClient).
	HTTPClient *http.Client

	// Logger is used for error logging. If nil, the client's logger
	// is used.
	Logger *slog.Logger

	// DedupeCacheSize is how many recent "Experiment Viewed" and "Feature
	// Evaluated" events the plugin remembers to drop repeats. Defaults to
	// 1000; a negative value disables deduplication.
	DedupeCacheSize int

	// DedupeKeyAttributes are the attributes that identify a user when
	// deduplicating events. When empty, "Experiment Viewed" events are
	// identified by their hash attribute and value (one exposure per
	// bucketing unit) and "Feature Evaluated" events by all of the
	// evaluating client's attributes (one evaluation per user).
	DedupeKeyAttributes []string
}

// trackingEvent is the JSON payload for a single tracking event, in the
// ingestor's EventPayload shape (same as the JS/Python tracking plugins).
type trackingEvent map[string]any

// GrowthBookTrackingPlugin sends experiment and feature evaluation
// events to the GrowthBook ingestor for warehouse analytics.
type GrowthBookTrackingPlugin struct {
	config     TrackingPluginConfig
	clientKey  string
	httpClient *http.Client
	logger     *slog.Logger

	initialized bool

	mu     sync.Mutex
	events []json.RawMessage
	timer  *time.Timer
	closed bool
	wg     sync.WaitGroup // tracks in-flight background sends

	dedupe *eventDedupeCache // nil when deduplication is disabled
}

// NewGrowthBookTrackingPlugin creates a new tracking plugin with the
// given configuration. The plugin must be passed to WithPlugins or
// WithGrowthBookTracking when creating a client.
func NewGrowthBookTrackingPlugin(config TrackingPluginConfig) *GrowthBookTrackingPlugin {
	if config.IngestorHost == "" {
		config.IngestorHost = defaultIngestorHost
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaultBatchSize
	}
	if config.BatchTimeout <= 0 {
		config.BatchTimeout = defaultBatchTimeout
	}
	if config.DedupeCacheSize == 0 {
		config.DedupeCacheSize = defaultDedupeCacheSize
	}
	p := &GrowthBookTrackingPlugin{
		config: config,
	}
	if config.DedupeCacheSize > 0 {
		p.dedupe = newEventDedupeCache(config.DedupeCacheSize)
	}
	return p
}

// Init initializes the plugin with the client's configuration.
// If initialization fails the plugin remains uninitialised and all
// tracking calls become no-ops — SDK evaluation is never affected.
func (p *GrowthBookTrackingPlugin) Init(client *Client) error {
	p.clientKey = client.ClientKey()
	if p.clientKey == "" {
		return fmt.Errorf("growthbook tracking plugin requires a client key")
	}

	if p.config.HTTPClient != nil {
		p.httpClient = p.config.HTTPClient
	} else {
		p.httpClient = client.HttpClient()
	}

	if p.config.Logger != nil {
		p.logger = p.config.Logger
	} else {
		p.logger = client.Logger()
	}

	p.initialized = true
	return nil
}

// OnExperimentViewed is a no-op: built-in "Experiment Viewed" events
// arrive via OnEvent through the client's event-logger dispatch, which
// carries the user context the ingestor payload needs. Retained to
// satisfy the Plugin interface.
func (p *GrowthBookTrackingPlugin) OnExperimentViewed(ctx context.Context, experiment *Experiment, result *ExperimentResult) {
}

// OnFeatureEvaluated is a no-op: built-in "Feature Evaluated" events
// arrive via OnEvent through the client's event-logger dispatch.
// Retained to satisfy the Plugin interface.
func (p *GrowthBookTrackingPlugin) OnFeatureEvaluated(ctx context.Context, featureKey string, result *FeatureResult) {
}

// OnEvent enqueues an event from the client's event-logger dispatch: the
// built-in Experiment Viewed / Feature Evaluated events and custom events
// logged via [Client.LogEvent]. The payload mirrors the EventPayload shape
// used by the JS and Python tracking plugins (sdk-js
// plugins/growthbook-tracking.ts getEventPayload).
// No-op if the plugin was not successfully initialized.
func (p *GrowthBookTrackingPlugin) OnEvent(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext) {
	if !p.initialized {
		return
	}
	if properties == nil {
		properties = EventProperties{}
	}
	if p.isDuplicate(eventName, properties, userCtx.Attributes) {
		return
	}
	event := trackingEvent{
		"event_name":      eventName,
		"properties_json": properties,
		"sdk_language":    "go",
		"sdk_version":     sdkVersion(),
		"url":             userCtx.URL,
	}
	addEventAttributes(event, userCtx.Attributes)
	p.enqueue(event)
}

// isDuplicate reports whether an equal "Experiment Viewed" or "Feature
// Evaluated" event was logged recently, mirroring the JS tracking plugin's
// de-duping (sdk-js plugins/growthbook-tracking.ts). The plugin is shared by
// a client and all its child clients, so it always applies the JS
// multi-user rule: "Feature Evaluated" carries no bucketing unit, so without
// DedupeKeyAttributes it is identified by all of the user's attributes —
// otherwise identical evaluations by different users would collapse into
// one. "Experiment Viewed" properties already hold the hash attribute and
// value, so exposures stay one per bucketing unit, not per user.
func (p *GrowthBookTrackingPlugin) isDuplicate(eventName string, properties EventProperties, attributes Attributes) bool {
	if p.dedupe == nil || (eventName != EventFeatureEvaluated && eventName != EventExperimentViewed) {
		return false
	}
	keyData := map[string]any{
		"eventName":  eventName,
		"properties": properties,
	}
	if len(p.config.DedupeKeyAttributes) > 0 {
		for _, k := range p.config.DedupeKeyAttributes {
			if v, ok := attributes[k]; ok {
				keyData["attr:"+k] = v
			}
		}
	} else if eventName == EventFeatureEvaluated {
		for k, v := range attributes {
			keyData["attr:"+k] = v
		}
	}
	data, err := json.Marshal(keyData)
	if err != nil {
		// enqueue reports the unmarshalable event; don't drop it here.
		return false
	}
	// Remember a digest, not the key: the cache never holds attribute values.
	return p.dedupe.seen(sha256.Sum256(data))
}

// eventDedupeCache is an LRU set of recently logged event keys, shared by
// every client using the plugin, so access is mutex-guarded.
type eventDedupeCache struct {
	mu      sync.Mutex
	maxSize int
	keys    map[[sha256.Size]byte]*list.Element
	order   *list.List // front = most recently seen
}

func newEventDedupeCache(maxSize int) *eventDedupeCache {
	return &eventDedupeCache{
		maxSize: maxSize,
		keys:    make(map[[sha256.Size]byte]*list.Element),
		order:   list.New(),
	}
}

// seen reports whether key is in the cache and records it as most recently
// seen, evicting the least recently seen key when over capacity.
func (c *eventDedupeCache) seen(key [sha256.Size]byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.keys[key]; ok {
		c.order.MoveToFront(elem)
		return true
	}
	c.keys[key] = c.order.PushFront(key)
	if c.order.Len() > c.maxSize {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.keys, oldest.Value.([sha256.Size]byte))
	}
	return false
}

// Attribute keys lifted to top-level EventPayload fields; everything else
// goes into context_json. Mirrors parseAttributes in the JS tracking plugin.
var (
	eventIDAttrKeys = map[string]string{
		"user_id":    "user_id",
		"page_id":    "page_id",
		"session_id": "session_id",
	}
	eventOptionalAttrKeys = map[string]string{
		"utmCampaign": "utm_campaign",
		"utmContent":  "utm_content",
		"utmMedium":   "utm_medium",
		"utmSource":   "utm_source",
		"utmTerm":     "utm_term",
		"pageTitle":   "page_title",
	}
	eventDeviceIDAttrKeys = []string{"device_id", "anonymous_id", "id"}
)

// addEventAttributes splits attributes into the top-level EventPayload
// fields and the context_json object, like the JS plugin's parseAttributes:
// id fields are always present (scalars stringified, null otherwise — see
// identifierString), device_id falls back to anonymous_id then id, and
// UTM/page_title fields are only set when they hold a string.
func addEventAttributes(event trackingEvent, attributes Attributes) {
	nested := make(map[string]any, len(attributes))
	for k, v := range attributes {
		if _, ok := eventIDAttrKeys[k]; ok {
			continue
		}
		if _, ok := eventOptionalAttrKeys[k]; ok {
			continue
		}
		if k == "device_id" || k == "anonymous_id" || k == "id" {
			continue
		}
		nested[k] = v
	}
	event["context_json"] = nested

	for attrKey, field := range eventIDAttrKeys {
		event[field] = identifierString(attributes[attrKey])
	}
	event["device_id"] = nil
	for _, k := range eventDeviceIDAttrKeys {
		if truthy(attributes[k]) {
			event["device_id"] = identifierString(attributes[k])
			break
		}
	}
	for attrKey, field := range eventOptionalAttrKeys {
		if s, ok := attributes[attrKey].(string); ok && s != "" {
			event[field] = s
		}
	}
}

// identifierString returns an identifier attribute's value for the event
// payload: strings pass through, numeric and boolean scalars are stringified
// with the SDK's JS-toString semantics, everything else is nil. The JS
// plugin's parseString nulls all non-strings, but Go attributes commonly
// carry numeric ids (the README's examples do) and the SDK already
// stringifies them for hashing — nulling them here would strip attribution
// from every event. A deliberate, documented divergence from the JS plugin.
func identifierString(v any) any {
	val := value.New(v)
	switch val.Type() {
	case value.StrType, value.NumType, value.BoolType:
		return val.String()
	default:
		return nil
	}
}

// Close flushes any remaining events and releases resources. Safe to
// call multiple times.
func (p *GrowthBookTrackingPlugin) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	events := p.events
	p.events = nil
	p.mu.Unlock()

	// Wait for any in-flight background sends to complete.
	p.wg.Wait()

	if len(events) > 0 {
		// Synchronous flush on close with a reasonable timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.sendBatch(ctx, events)
	}
	return nil
}

// enqueue adds an event to the batch. If the batch is full, it
// triggers an immediate background flush. If this is the first event
// in a new batch, it starts the timeout timer.
// Events are marshaled here, one by one, so a single unmarshalable
// value (e.g. NaN in LogEvent properties) drops only that event
// instead of failing the whole batch at send time.
func (p *GrowthBookTrackingPlugin) enqueue(event trackingEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		p.logger.Error("Failed to marshal tracking event, dropping it", "error", err)
		return
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}

	p.events = append(p.events, data)

	if len(p.events) >= p.config.BatchSize {
		// Batch full — flush immediately.
		events := p.events
		p.events = nil
		if p.timer != nil {
			p.timer.Stop()
			p.timer = nil
		}
		// wg.Add must happen before unlock so Close's wg.Wait cannot
		// return while a send goroutine is about to be launched.
		p.wg.Add(1)
		p.mu.Unlock()
		go func() {
			defer p.wg.Done()
			p.sendBatch(context.Background(), events)
		}()
		return
	}

	// Start timer if this is the first event in a new batch.
	if p.timer == nil {
		p.timer = time.AfterFunc(p.config.BatchTimeout, p.timerFlush)
	}
	p.mu.Unlock()
}

// timerFlush is called by the batch timeout timer.
func (p *GrowthBookTrackingPlugin) timerFlush() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	events := p.events
	p.events = nil
	p.timer = nil
	// wg.Add must happen before unlock — same reasoning as enqueue.
	if len(events) > 0 {
		p.wg.Add(1)
	}
	p.mu.Unlock()

	if len(events) > 0 {
		go func() {
			defer p.wg.Done()
			p.sendBatch(context.Background(), events)
		}()
	}
}

// sendBatch POSTs a batch of events to the ingestor: a bare JSON array to
// {host}/track?client_key={key}, the contract the JS and Python SDKs use
// and the only one the ingestor documents. Content-Type text/plain matches
// those SDKs' requests. Delivery is best-effort: transient errors are
// logged and the batch is dropped. No retry is attempted to keep the
// implementation simple and avoid unbounded memory growth or complex
// retry state.
func (p *GrowthBookTrackingPlugin) sendBatch(ctx context.Context, events []json.RawMessage) {
	body, err := json.Marshal(events)
	if err != nil {
		p.logger.Error("Failed to marshal tracking events", "error", err)
		return
	}

	endpoint := p.config.IngestorHost + "/track?client_key=" + url.QueryEscape(p.clientKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		p.logger.Error("Failed to create tracking request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("growthbook-go-sdk/%s", sdkVersion()))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Error("Failed to send tracking events", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.logger.Error("Tracking ingestor returned non-success status", "status", resp.StatusCode)
	}
}
