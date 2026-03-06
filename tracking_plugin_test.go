package growthbook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// capturedRequest holds the parsed body from a tracking POST.
type capturedRequest struct {
	ClientKey string          `json:"client_key"`
	Events    []trackingEvent `json:"events"`
}

// newTestIngestor creates an httptest server that captures tracking requests.
func newTestIngestor(t *testing.T) (*httptest.Server, *[]capturedRequest, *sync.Mutex) {
	t.Helper()
	var reqs []capturedRequest
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var req capturedRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("failed to unmarshal body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		reqs = append(reqs, req)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	return srv, &reqs, &mu
}

func getRequests(reqs *[]capturedRequest, mu *sync.Mutex) []capturedRequest {
	mu.Lock()
	defer mu.Unlock()
	result := make([]capturedRequest, len(*reqs))
	copy(result, *reqs)
	return result
}

func TestTrackingPluginExperimentViewed(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-123"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1, // flush on every event for test
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{
		"exp-feature": {
			"defaultValue": 0,
			"rules": [{"variations": [0, 1], "name": "My Experiment"}]
		}
	}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	res := client.EvalFeature(ctx, "exp-feature")
	require.Equal(t, 1.0, res.Value)
	require.True(t, res.InExperiment())

	// Close to ensure flush completes.
	require.NoError(t, client.Close())

	captured := getRequests(reqs, mu)
	require.NotEmpty(t, captured)

	// Collect all events across batches.
	var allEvents []trackingEvent
	for _, batch := range captured {
		require.Equal(t, "sdk-test-key", batch.ClientKey)
		allEvents = append(allEvents, batch.Events...)
	}

	// Should have both feature_evaluated and experiment_viewed.
	var expEvent, featEvent trackingEvent
	for _, e := range allEvents {
		switch e["event_type"] {
		case eventExperimentViewed:
			expEvent = e
		case eventFeatureEvaluated:
			featEvent = e
		}
	}

	require.NotNil(t, expEvent, "expected experiment_viewed event")
	require.Equal(t, "exp-feature", expEvent["experiment_id"])
	require.Equal(t, "go", expEvent["sdk_language"])
	require.Equal(t, true, expEvent["in_experiment"])
	require.Equal(t, true, expEvent["hash_used"])
	require.Equal(t, "id", expEvent["hash_attribute"])
	require.Equal(t, "user-123", expEvent["hash_value"])

	require.NotNil(t, featEvent, "expected feature_evaluated event")
	require.Equal(t, "exp-feature", featEvent["feature_key"])
	require.Equal(t, "experiment", featEvent["source"])
}

func TestTrackingPluginFeatureEvaluated(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-456"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1,
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{"simple-flag": {"defaultValue": true}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	res := client.EvalFeature(ctx, "simple-flag")
	require.Equal(t, true, res.Value)

	require.NoError(t, client.Close())

	captured := getRequests(reqs, mu)
	require.NotEmpty(t, captured)

	var allEvents []trackingEvent
	for _, batch := range captured {
		allEvents = append(allEvents, batch.Events...)
	}

	require.Len(t, allEvents, 1)
	event := allEvents[0]
	require.Equal(t, eventFeatureEvaluated, event["event_type"])
	require.Equal(t, "simple-flag", event["feature_key"])
	require.Equal(t, true, event["feature_value"])
	require.Equal(t, "defaultValue", event["source"])
	require.Equal(t, true, event["on"])
	require.Equal(t, false, event["off"])
}

func TestTrackingPluginBatching(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-batch"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    5,
			BatchTimeout: 1 * time.Hour, // effectively disable timer
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{"flag": {"defaultValue": true}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	// Evaluate 4 times — should NOT trigger flush yet (batch size is 5).
	for i := 0; i < 4; i++ {
		client.EvalFeature(ctx, "flag")
	}

	// Give any background goroutines time to run.
	time.Sleep(50 * time.Millisecond)
	captured := getRequests(reqs, mu)
	require.Empty(t, captured, "should not flush before batch size is reached")

	// 5th evaluation triggers flush (batch size 5 reached).
	client.EvalFeature(ctx, "flag")

	// Wait for background flush goroutine.
	time.Sleep(100 * time.Millisecond)
	captured = getRequests(reqs, mu)
	require.Len(t, captured, 1, "should flush once when batch size is reached")
	require.Len(t, captured[0].Events, 5)

	require.NoError(t, client.Close())
}

func TestTrackingPluginBatchTimeout(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-timeout"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    100, // high enough to not trigger size-based flush
			BatchTimeout: 100 * time.Millisecond,
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{"flag": {"defaultValue": true}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	client.EvalFeature(ctx, "flag")

	// Should not have flushed immediately.
	time.Sleep(10 * time.Millisecond)
	captured := getRequests(reqs, mu)
	require.Empty(t, captured, "should not flush before timeout")

	// Wait for timeout to trigger flush.
	time.Sleep(200 * time.Millisecond)
	captured = getRequests(reqs, mu)
	require.Len(t, captured, 1, "should flush after timeout")
	require.Len(t, captured[0].Events, 1)

	require.NoError(t, client.Close())
}

func TestTrackingPluginCloseFlushesRemaining(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-close"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    100,              // won't trigger on size
			BatchTimeout: 1 * time.Hour,    // won't trigger on time
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{"flag": {"defaultValue": 42}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	client.EvalFeature(ctx, "flag")
	client.EvalFeature(ctx, "flag")
	client.EvalFeature(ctx, "flag")

	// Nothing should be flushed yet.
	captured := getRequests(reqs, mu)
	require.Empty(t, captured)

	// Close flushes remaining events synchronously.
	require.NoError(t, client.Close())

	captured = getRequests(reqs, mu)
	require.Len(t, captured, 1)
	require.Len(t, captured[0].Events, 3)
}

func TestTrackingPluginCloseIdempotent(t *testing.T) {
	srv, _, _ := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
		}),
	)
	require.NoError(t, err)

	// Closing multiple times should not panic.
	require.NoError(t, client.Close())
	require.NoError(t, client.Close())
}

func TestTrackingPluginNoClientKeyIsNoOp(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	// No WithClientKey — plugin Init will fail, but client creation succeeds.
	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "user-no-key"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1,
		}),
	)
	require.NoError(t, err, "client should be created even if plugin init fails")

	featuresJSON := `{"flag": {"defaultValue": true}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	// EvalFeature should work fine — uninitialized plugin is a no-op.
	res := client.EvalFeature(ctx, "flag")
	require.Equal(t, true, res.Value)

	require.NoError(t, client.Close())

	// No events should have been sent.
	captured := getRequests(reqs, mu)
	require.Empty(t, captured, "uninitialized plugin should not send events")
}

func TestTrackingPluginRunExperiment(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-exp"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1,
		}),
	)
	require.NoError(t, err)

	exp := &Experiment{
		Key:        "my-experiment",
		Variations: []FeatureValue{"control", "variant"},
	}
	result := client.RunExperiment(ctx, exp)
	require.True(t, result.InExperiment)

	require.NoError(t, client.Close())

	captured := getRequests(reqs, mu)
	require.NotEmpty(t, captured)

	var allEvents []trackingEvent
	for _, batch := range captured {
		allEvents = append(allEvents, batch.Events...)
	}

	// RunExperiment only triggers experiment_viewed, not feature_evaluated.
	var expEvents []trackingEvent
	for _, e := range allEvents {
		if e["event_type"] == eventExperimentViewed {
			expEvents = append(expEvents, e)
		}
	}
	require.NotEmpty(t, expEvents)
	require.Equal(t, "my-experiment", expEvents[0]["experiment_id"])
}

func TestTrackingPluginWithExistingCallbacks(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	callbackCalled := false
	featureCbCalled := false
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-cb"}),
		WithExperimentCallback(func(ctx context.Context, exp *Experiment, result *ExperimentResult, extraData any) {
			callbackCalled = true
		}),
		WithFeatureUsageCallback(func(ctx context.Context, key string, result *FeatureResult, extraData any) {
			featureCbCalled = true
		}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1,
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{
		"exp-feature": {
			"defaultValue": 0,
			"rules": [{"variations": [0, 1]}]
		}
	}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	client.EvalFeature(ctx, "exp-feature")
	require.NoError(t, client.Close())

	// Both existing callbacks and plugin should have fired.
	require.True(t, callbackCalled, "experiment callback should still fire")
	require.True(t, featureCbCalled, "feature usage callback should still fire")

	captured := getRequests(reqs, mu)
	require.NotEmpty(t, captured, "plugin should have sent events too")
}

func TestTrackingPluginChildClientSharesPlugin(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "parent-user"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    10,
			BatchTimeout: 1 * time.Hour,
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{"flag": {"defaultValue": true}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	child, err := client.WithAttributes(Attributes{"id": "child-user"})
	require.NoError(t, err)

	// Both parent and child evaluations should go to the same plugin.
	client.EvalFeature(ctx, "flag")
	child.EvalFeature(ctx, "flag")

	// Close from parent should flush all events including child's.
	require.NoError(t, client.Close())

	captured := getRequests(reqs, mu)
	require.NotEmpty(t, captured)

	var allEvents []trackingEvent
	for _, batch := range captured {
		allEvents = append(allEvents, batch.Events...)
	}
	require.Len(t, allEvents, 2, "both parent and child events should be captured")
}

func TestTrackingPluginPanicRecovery(t *testing.T) {
	ctx := context.Background()

	// Create a plugin that panics on every call.
	panicPlugin := &panickyPlugin{}

	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-panic"}),
		WithPlugins(panicPlugin),
	)
	require.NoError(t, err)

	featuresJSON := `{"flag": {"defaultValue": true}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	// Should not panic — the plugin's panic is recovered.
	res := client.EvalFeature(ctx, "flag")
	require.Equal(t, true, res.Value)

	require.NoError(t, client.Close())
}

// panickyPlugin is a test plugin that panics on tracking calls.
type panickyPlugin struct{}

func (p *panickyPlugin) Init(client *Client) error                { return nil }
func (p *panickyPlugin) Close() error                             { return nil }
func (p *panickyPlugin) OnExperimentViewed(ctx context.Context, exp *Experiment, res *ExperimentResult) {
	panic("experiment viewed panic")
}
func (p *panickyPlugin) OnFeatureEvaluated(ctx context.Context, key string, res *FeatureResult) {
	panic("feature evaluated panic")
}
