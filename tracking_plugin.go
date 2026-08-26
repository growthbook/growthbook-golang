package growthbook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

const (
	defaultIngestorHost = "https://us1.gb-ingest.com"
	defaultBatchSize    = 100
	defaultBatchTimeout = 10 * time.Second

	eventExperimentViewed = "experiment_viewed"
	eventFeatureEvaluated = "feature_evaluated"
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
}

// trackingEvent is the JSON payload for a single tracking event.
type trackingEvent map[string]any

// trackingRequest is the JSON body sent to the ingestor.
type trackingRequest struct {
	Events    []trackingEvent `json:"events"`
	ClientKey string          `json:"client_key"`
}

// GrowthBookTrackingPlugin sends experiment and feature evaluation
// events to the GrowthBook ingestor for warehouse analytics.
type GrowthBookTrackingPlugin struct {
	config     TrackingPluginConfig
	clientKey  string
	httpClient *http.Client
	logger     *slog.Logger

	initialized bool

	mu     sync.Mutex
	events []trackingEvent
	timer  *time.Timer
	closed bool
	wg     sync.WaitGroup // tracks in-flight background sends
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
	return &GrowthBookTrackingPlugin{
		config: config,
	}
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

// OnExperimentViewed enqueues an experiment_viewed event.
// No-op if the plugin was not successfully initialized.
func (p *GrowthBookTrackingPlugin) OnExperimentViewed(ctx context.Context, experiment *Experiment, result *ExperimentResult) {
	if !p.initialized {
		return
	}
	event := trackingEvent{
		"event_type":      eventExperimentViewed,
		"timestamp":       time.Now().UnixMilli(),
		"sdk_language":    "go",
		"sdk_version":     sdkVersion(),
		"experiment_id":   experiment.Key,
		"variation_id":    result.VariationId,
		"variation_key":   result.Key,
		"variation_value": result.Value,
		"in_experiment":   result.InExperiment,
		"hash_used":       result.HashUsed,
		"hash_attribute":  result.HashAttribute,
		"hash_value":      result.HashValue,
	}
	if experiment.Name != "" {
		event["experiment_name"] = experiment.Name
	}
	if result.FeatureId != "" {
		event["feature_id"] = result.FeatureId
	}
	p.enqueue(event)
}

// OnFeatureEvaluated enqueues a feature_evaluated event.
// No-op if the plugin was not successfully initialized.
func (p *GrowthBookTrackingPlugin) OnFeatureEvaluated(ctx context.Context, featureKey string, result *FeatureResult) {
	if !p.initialized {
		return
	}
	event := trackingEvent{
		"event_type":    eventFeatureEvaluated,
		"timestamp":     time.Now().UnixMilli(),
		"sdk_language":  "go",
		"sdk_version":   sdkVersion(),
		"feature_key":   featureKey,
		"feature_value": result.Value,
		"source":        string(result.Source),
		"on":            result.On,
		"off":           result.Off,
	}
	if result.RuleId != "" {
		event["rule_id"] = result.RuleId
	}
	if result.Experiment != nil {
		event["experiment_id"] = result.Experiment.Key
	}
	if result.ExperimentResult != nil {
		event["variation_id"] = result.ExperimentResult.VariationId
	}
	p.enqueue(event)
}

// OnEvent enqueues a custom event logged via [Client.LogEvent]. The
// payload mirrors the EventPayload shape used by the JS and Python
// tracking plugins (sdk-js plugins/growthbook-tracking.ts getEventPayload).
// No-op if the plugin was not successfully initialized.
func (p *GrowthBookTrackingPlugin) OnEvent(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext) {
	if !p.initialized {
		return
	}
	if properties == nil {
		properties = EventProperties{}
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
// id fields are always present (null when not a string), device_id falls
// back to anonymous_id then id, and UTM/page_title fields are only set
// when they hold a string.
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
		event[field] = stringOrNil(attributes[attrKey])
	}
	event["device_id"] = nil
	for _, k := range eventDeviceIDAttrKeys {
		if truthy(attributes[k]) {
			event["device_id"] = stringOrNil(attributes[k])
			break
		}
	}
	for attrKey, field := range eventOptionalAttrKeys {
		if s, ok := attributes[attrKey].(string); ok && s != "" {
			event[field] = s
		}
	}
}

// stringOrNil returns the value if it is a string, otherwise nil —
// like the JS plugin's parseString.
func stringOrNil(v any) any {
	if s, ok := v.(string); ok {
		return s
	}
	return nil
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
func (p *GrowthBookTrackingPlugin) enqueue(event trackingEvent) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}

	p.events = append(p.events, event)

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

// sendBatch POSTs a batch of events to the ingestor endpoint.
// Delivery is best-effort: transient errors are logged and the batch
// is dropped. No retry is attempted to keep the implementation simple
// and avoid unbounded memory growth or complex retry state.
func (p *GrowthBookTrackingPlugin) sendBatch(ctx context.Context, events []trackingEvent) {
	body, err := json.Marshal(trackingRequest{
		Events:    events,
		ClientKey: p.clientKey,
	})
	if err != nil {
		p.logger.Error("Failed to marshal tracking events", "error", err)
		return
	}

	url := p.config.IngestorHost + "/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		p.logger.Error("Failed to create tracking request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
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
