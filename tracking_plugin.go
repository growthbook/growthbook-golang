package growthbook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
		"client_key":      p.clientKey,
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
		"client_key":    p.clientKey,
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
		p.mu.Unlock()
		p.wg.Add(1)
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
	events := p.events
	p.events = nil
	p.timer = nil
	p.mu.Unlock()

	if len(events) > 0 {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.sendBatch(context.Background(), events)
		}()
	}
}

// sendBatch POSTs a batch of events to the ingestor endpoint.
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
