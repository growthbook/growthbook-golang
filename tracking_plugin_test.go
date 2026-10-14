package growthbook

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// capturedRequest holds one tracking POST: the request metadata the
// ingestor contract requires plus the parsed bare-array body.
type capturedRequest struct {
	Path        string
	ClientKey   string
	ContentType string
	Events      []trackingEvent
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
		var events []trackingEvent
		if err := json.Unmarshal(body, &events); err != nil {
			t.Errorf("body is not a bare JSON array of events: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		reqs = append(reqs, capturedRequest{
			Path:        r.URL.Path,
			ClientKey:   r.URL.Query().Get("client_key"),
			ContentType: r.Header.Get("Content-Type"),
			Events:      events,
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	return srv, &reqs, &mu
}

// eventsByName splits captured events by event_name, asserting the wire
// contract on every request: POST /track, client_key in the query string,
// text/plain content type.
func eventsByName(t *testing.T, captured []capturedRequest, clientKey string) map[string][]trackingEvent {
	t.Helper()
	out := make(map[string][]trackingEvent)
	for _, req := range captured {
		require.Equal(t, "/track", req.Path)
		require.Equal(t, clientKey, req.ClientKey)
		require.Equal(t, "text/plain", req.ContentType)
		for _, e := range req.Events {
			name, _ := e["event_name"].(string)
			require.NotEmpty(t, name, "every event must carry event_name")
			out[name] = append(out[name], e)
		}
	}
	return out
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
	byName := eventsByName(t, captured, "sdk-test-key")

	// One exposure evaluation yields exactly one of each built-in event.
	require.Len(t, byName[EventExperimentViewed], 1, "expected one Experiment Viewed event")
	require.Len(t, byName[EventFeatureEvaluated], 1, "expected one Feature Evaluated event")

	expEvent := byName[EventExperimentViewed][0]
	require.Equal(t, map[string]any{
		"experimentId":  "exp-feature",
		"variationId":   "1", // variation key, not the numeric index
		"hashAttribute": "id",
		"hashValue":     "user-123",
	}, expEvent["properties_json"])
	require.Equal(t, "go", expEvent["sdk_language"])
	// "id" is lifted to device_id; nothing remains for context_json.
	require.Equal(t, "user-123", expEvent["device_id"])
	require.Contains(t, expEvent, "user_id")
	require.Nil(t, expEvent["user_id"])
	require.Equal(t, map[string]any{}, expEvent["context_json"])
	require.NotContains(t, expEvent, "event_type")
	require.NotContains(t, expEvent, "client_key")

	featEvent := byName[EventFeatureEvaluated][0]
	require.Equal(t, map[string]any{
		"feature":     "exp-feature",
		"source":      "experiment",
		"value":       1.0,
		"ruleId":      "",
		"variationId": "1",
	}, featEvent["properties_json"])
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

	byName := eventsByName(t, captured, "sdk-test-key")
	require.Len(t, byName[EventFeatureEvaluated], 1)
	require.Empty(t, byName[EventExperimentViewed])

	event := byName[EventFeatureEvaluated][0]
	// $default marks defaultValue-sourced evaluations, matching the JS SDK.
	require.Equal(t, map[string]any{
		"feature":     "simple-flag",
		"source":      "defaultValue",
		"value":       true,
		"ruleId":      "$default",
		"variationId": "",
	}, event["properties_json"])
	require.Equal(t, "user-456", event["device_id"])
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

	// Give any background goroutines time to run (none should be running).
	time.Sleep(50 * time.Millisecond)
	captured := getRequests(reqs, mu)
	require.Empty(t, captured, "should not flush before batch size is reached")

	// 5th evaluation triggers flush (batch size 5 reached).
	client.EvalFeature(ctx, "flag")

	// Wait for background flush goroutine with a deadline.
	require.Eventually(t, func() bool {
		return len(getRequests(reqs, mu)) == 1
	}, 2*time.Second, 10*time.Millisecond, "should flush once when batch size is reached")

	captured = getRequests(reqs, mu)
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
	require.Eventually(t, func() bool {
		return len(getRequests(reqs, mu)) == 1
	}, 2*time.Second, 10*time.Millisecond, "should flush after timeout")

	captured = getRequests(reqs, mu)
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
			BatchSize:    100,           // won't trigger on size
			BatchTimeout: 1 * time.Hour, // won't trigger on time
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
	require.Len(t, captured, 1)
	byName := eventsByName(t, captured, "sdk-test-key")

	// RunExperiment produces exactly one event: Experiment Viewed.
	// Feature Evaluated is only emitted by EvalFeature.
	require.Len(t, byName[EventExperimentViewed], 1)
	require.Empty(t, byName[EventFeatureEvaluated])
	props := byName[EventExperimentViewed][0]["properties_json"].(map[string]any)
	require.Equal(t, "my-experiment", props["experimentId"])
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
		WithExperimentCallback(func(ctx context.Context, exp *Experiment, result *ExperimentResult, userCtx *TrackingUserContext, extraData any) {
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

	byName := eventsByName(t, getRequests(reqs, mu), "sdk-test-key")
	require.NotEmpty(t, byName[EventExperimentViewed], "plugin should have sent Experiment Viewed")
	require.NotEmpty(t, byName[EventFeatureEvaluated], "plugin should have sent Feature Evaluated")
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

// TestTrackingPluginCloseRace verifies that no events are lost when a
// size-triggered batch flush races with Close. Specifically it guards
// the wg.Add-before-unlock ordering fixed in enqueue and timerFlush.
func TestTrackingPluginCloseRace(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "user-race"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    5,
			BatchTimeout: 1 * time.Hour,
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{"flag": {"defaultValue": true}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	// Pre-fill 3 events synchronously before the race. Because these are
	// already in p.events when the race starts, they are guaranteed to be
	// delivered: either Close's synchronous flush sends them, or they land
	// in a size-triggered batch flush that wg.Wait will wait for.
	for i := 0; i < 3; i++ {
		client.EvalFeature(ctx, "flag")
	}

	// Race: add 2 more events (which may trigger the BatchSize=5 flush)
	// concurrently with Close.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2; i++ {
			client.EvalFeature(ctx, "flag")
		}
	}()
	go func() {
		defer wg.Done()
		client.Close()
	}()
	wg.Wait()

	var total int
	for _, batch := range getRequests(reqs, mu) {
		total += len(batch.Events)
	}
	// Lower bound: the 3 pre-race events must always be delivered.
	// Upper bound: no event can be double-sent (mutex prevents it).
	require.GreaterOrEqual(t, total, 3, "pre-race events must never be lost")
	require.LessOrEqual(t, total, 5, "no event should be sent more than once")
}

// panickyPlugin is a test plugin that panics on tracking calls.
type panickyPlugin struct{}

func (p *panickyPlugin) Init(client *Client) error { return nil }
func (p *panickyPlugin) Close() error              { return nil }
func (p *panickyPlugin) OnExperimentViewed(ctx context.Context, exp *Experiment, res *ExperimentResult) {
	panic("experiment viewed panic")
}
func (p *panickyPlugin) OnFeatureEvaluated(ctx context.Context, key string, res *FeatureResult) {
	panic("feature evaluated panic")
}

func TestTrackingPluginLogEvent(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{
			"id":         "user-123",
			"user_id":    "u-9",
			"utmSource":  "newsletter",
			"utmContent": 42,   // non-string — omitted
			"pageTitle":  "",   // empty string — omitted
			"country":    "US", // nested
		}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1, // flush on every event for test
		}),
	)
	require.NoError(t, err)

	client.LogEvent(ctx, "button_clicked", EventProperties{"button": "buy"})
	require.NoError(t, client.Close())

	captured := getRequests(reqs, mu)
	require.Len(t, captured, 1)
	require.Equal(t, "sdk-test-key", captured[0].ClientKey)
	require.Len(t, captured[0].Events, 1)

	event := captured[0].Events[0]
	require.Equal(t, "button_clicked", event["event_name"])
	require.Equal(t, map[string]any{"button": "buy"}, event["properties_json"])
	require.Equal(t, "go", event["sdk_language"])
	require.Equal(t, "", event["url"])

	// Top-level id fields: user_id from attributes, device_id falls back
	// to id, page_id/session_id null when absent.
	require.Equal(t, "u-9", event["user_id"])
	require.Equal(t, "user-123", event["device_id"])
	require.Contains(t, event, "page_id")
	require.Nil(t, event["page_id"])
	require.Contains(t, event, "session_id")
	require.Nil(t, event["session_id"])

	// UTM fields: string values pass through, non-string and empty
	// string values are omitted.
	require.Equal(t, "newsletter", event["utm_source"])
	require.NotContains(t, event, "utm_content")
	require.NotContains(t, event, "page_title")

	// Remaining attributes land in context_json; lifted keys don't.
	require.Equal(t, map[string]any{"country": "US"}, event["context_json"])
}

func TestTrackingPluginLogEventNilProperties(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1,
		}),
	)
	require.NoError(t, err)

	client.LogEvent(ctx, "bare_event", nil)
	require.NoError(t, client.Close())

	captured := getRequests(reqs, mu)
	require.Len(t, captured, 1)
	event := captured[0].Events[0]
	require.Equal(t, "bare_event", event["event_name"])
	require.Equal(t, map[string]any{}, event["properties_json"])
}

func TestTrackingPluginBadPropertyDropsOnlyThatEvent(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    10, // keep both events in one batch
		}),
		withSilentTestLogger(),
	)
	require.NoError(t, err)

	// NaN is rejected by json.Marshal; the event must be dropped at
	// enqueue time without poisoning the rest of the batch.
	client.LogEvent(ctx, "bad_event", EventProperties{"score": math.NaN()})
	client.LogEvent(ctx, "good_event", EventProperties{"ok": true})
	require.NoError(t, client.Close())

	captured := getRequests(reqs, mu)
	require.Len(t, captured, 1)
	require.Len(t, captured[0].Events, 1)
	require.Equal(t, "good_event", captured[0].Events[0]["event_name"])
}

// Built-in events flow through the event-logger channel, so a WithEventLogger
// callback receives them even without the tracking plugin — matching the JS
// SDK, where eventLogger gets "Experiment Viewed"/"Feature Evaluated" too.
func TestBuiltInEventsReachEventLogger(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	got := map[string][]EventProperties{}
	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "user-123"}),
		WithEventLogger(func(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext) {
			mu.Lock()
			got[eventName] = append(got[eventName], properties)
			mu.Unlock()
			require.NotNil(t, userCtx)
			require.Equal(t, Attributes{"id": "user-123"}, userCtx.Attributes)
		}),
		withSilentTestLogger(),
	)
	require.NoError(t, err)

	featuresJSON := `{
		"exp-feature": {
			"defaultValue": 0,
			"rules": [{"variations": [0, 1]}]
		}
	}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	res := client.EvalFeature(ctx, "exp-feature")
	require.True(t, res.InExperiment())

	require.Len(t, got[EventFeatureEvaluated], 1)
	require.Len(t, got[EventExperimentViewed], 1)
	require.Equal(t, "exp-feature", got[EventExperimentViewed][0]["experimentId"])
	require.Equal(t, res.ExperimentResult.Key, got[EventExperimentViewed][0]["variationId"])
}

// Events emitted from a child client's evaluation must carry the child's
// attributes, not the root client's.
func TestTrackingPluginChildClientAttributes(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": "parent-user", "user_id": "parent-user"}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1,
		}),
	)
	require.NoError(t, err)

	featuresJSON := `{"flag": {"defaultValue": true}}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	child, err := client.WithAttributes(Attributes{"id": "child-user", "user_id": "child-user"})
	require.NoError(t, err)
	child.EvalFeature(ctx, "flag")

	require.NoError(t, client.Close())

	byName := eventsByName(t, getRequests(reqs, mu), "sdk-test-key")
	require.Len(t, byName[EventFeatureEvaluated], 1)
	event := byName[EventFeatureEvaluated][0]
	require.Equal(t, "child-user", event["user_id"])
	require.Equal(t, "child-user", event["device_id"])
}

// Numeric identifier attributes must stringify into the id fields — the
// README documents numeric ids (Attributes{"id": 100}), and the SDK already
// stringifies them for hashing. (The JS plugin nulls non-strings; deliberate
// divergence.)
func TestTrackingPluginNumericIdentifiers(t *testing.T) {
	srv, reqs, mu := newTestIngestor(t)
	defer srv.Close()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey("sdk-test-key"),
		WithAttributes(Attributes{"id": 100, "user_id": 42}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: srv.URL,
			BatchSize:    1,
		}),
	)
	require.NoError(t, err)

	client.LogEvent(ctx, "numeric_ids", nil)
	require.NoError(t, client.Close())

	byName := eventsByName(t, getRequests(reqs, mu), "sdk-test-key")
	require.Len(t, byName["numeric_ids"], 1)
	event := byName["numeric_ids"][0]
	require.Equal(t, "42", event["user_id"])
	require.Equal(t, "100", event["device_id"])
}
