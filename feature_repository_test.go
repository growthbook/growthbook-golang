package growthbook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/r3labs/sse/v2"
)

type env struct {
	sync.RWMutex
	server       *httptest.Server
	sseServer    *sse.Server
	nullFeatures bool
	featureValue *string
	callCount    *int
	urls         map[string]int
}

func (e *env) checkCalls(t *testing.T, expected int) {
	e.RLock()
	defer e.RUnlock()
	if *e.callCount != expected {
		t.Errorf("Expected %d calls to API, got %d", expected, *e.callCount)
	}
}

func (e *env) close() {
	if e.sseServer != nil {
		e.sseServer.Close()
	}
	e.server.Close()
}

func setup(provideSSE bool) *env {
	return setupWithDelay(provideSSE, 50*time.Millisecond)
}

func setupWithDelay(provideSSE bool, delay time.Duration) *env {
	SetLogger(&testLog)
	testLog.reset()

	initialValue := "initial"
	callCount := 0
	urls := map[string]int{}
	env := env{featureValue: &initialValue, callCount: &callCount, urls: urls}

	// We need to set up a mock server to handle normal API requests and
	// SSE updates.
	mux := http.NewServeMux()

	// Normal GET.
	mux.HandleFunc("/api/features/", func(w http.ResponseWriter, r *http.Request) {
		env.Lock()
		defer env.Unlock()
		callCount++
		env.urls[r.URL.Path]++

		time.Sleep(delay)

		if provideSSE {
			w.Header().Set("X-SSE-Support", "enabled")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := &FeatureAPIResponse{
			Features: map[string]*Feature{
				"foo": {DefaultValue: *env.featureValue},
			},
		}
		if env.nullFeatures {
			response = nil
		}
		json.NewEncoder(w).Encode(response)
	})

	// SSE server handler.
	if provideSSE {
		env.sseServer = sse.New()
		env.sseServer.CreateStream("features")
		mux.HandleFunc("/sub/", env.sseServer.ServeHTTP)
	}

	env.server = httptest.NewServer(mux)

	return &env
}

func makeGB(apiHost string, clientKey string) *GrowthBook {
	context := NewContext().
		WithAPIHost(apiHost).
		WithClientKey(clientKey)
	return New(context)
}

func checkFeature(t *testing.T, gb *GrowthBook, feature string, expected interface{}) {
	value := gb.EvalFeature(feature).Value
	if value != expected {
		t.Errorf("feature value, expected %v, got %v", expected, value)
	}
}

func checkLogs(t *testing.T) {
	if len(testLog.errors) != 0 {
		t.Errorf("test log has errors: %s", testLog.allErrors())
	}
	if len(testLog.warnings) != 0 {
		t.Errorf("test log has warnings: %s", testLog.allWarnings())
	}
}

func knownWarnings(t *testing.T, count int) {
	if len(testLog.errors) != 0 {
		t.Error("found errors when looking for known warnings: ", testLog.allErrors())
		return
	}
	if len(testLog.warnings) == count {
		testLog.reset()
		return
	}

	t.Errorf("expected %d log warnings, got %d: %s", count,
		len(testLog.warnings), testLog.allWarnings())
}

func knownErrors(t *testing.T, messages ...string) {
	if len(testLog.errors) != len(messages) {
		t.Errorf("expected %d log errors, got %d: %s", len(messages),
			len(testLog.errors), testLog.allErrors())
		return
	}

	for i, msg := range messages {
		if !strings.HasPrefix(testLog.errors[i], msg) {
			t.Errorf("expected error message %d '%s...', got '%s'", i+1, msg, testLog.errors[i])
		}
	}

	testLog.reset()
}

func checkReady(t *testing.T, gb *GrowthBook, expected bool) {
	if gb.Ready() != expected {
		t.Errorf("expected ready flag to be %v", expected)
	}
}

func checkEmptyFeatures(t *testing.T, gb *GrowthBook) {
	if len(gb.Features()) != 0 {
		t.Error("expected feature map to be empty")
	}
}

func TestRepoDebounceFetchRequests(t *testing.T) {
	env := setup(false)
	defer cache.clear()
	defer checkLogs(t)
	defer env.close()

	cache.clear()

	gb1 := makeGB(env.server.URL, "qwerty1234")
	gb2 := makeGB(env.server.URL, "other")
	gb3 := makeGB(env.server.URL, "qwerty1234")

	gb1.LoadFeatures(nil)
	gb2.LoadFeatures(nil)
	gb3.LoadFeatures(nil)

	env.checkCalls(t, 2)
	if env.urls["/api/features/other"] != 1 ||
		env.urls["/api/features/qwerty1234"] != 1 {
		t.Errorf("unexpected URL calls: %v", env.urls)
	}

	checkFeature(t, gb1, "foo", "initial")
	checkFeature(t, gb2, "foo", "initial")
	checkFeature(t, gb3, "foo", "initial")
}

func TestRepoUsesCacheAndCanRefreshManually(t *testing.T) {
	env := setup(false)
	defer cache.clear()
	defer checkLogs(t)
	defer env.close()

	// Set cache TTL short so we can test expiry.
	savedCacheStaleTTL := cacheStaleTTL
	ConfigureCacheStaleTTL(100 * time.Millisecond)
	defer func() {
		ConfigureCacheStaleTTL(savedCacheStaleTTL)
	}()

	cache.clear()

	gb := makeGB(env.server.URL, "qwerty1234")
	time.Sleep(20 * time.Millisecond)

	// Initial value of feature should be null.
	checkFeature(t, gb, "foo", nil)
	env.checkCalls(t, 1)
	knownWarnings(t, 1)

	// Once features are loaded, value should be from the fetch request.
	gb.LoadFeatures(nil)
	checkFeature(t, gb, "foo", "initial")
	env.checkCalls(t, 1)

	// Value changes in API
	*env.featureValue = "changed"

	// New instances should get cached value
	gb2 := makeGB(env.server.URL, "qwerty1234")
	checkFeature(t, gb2, "foo", nil)
	knownWarnings(t, 1)
	gb2.LoadFeatures(&FeatureRepoOptions{AutoRefresh: true})
	checkFeature(t, gb2, "foo", "initial")

	// Instance without autoRefresh.
	gb3 := makeGB(env.server.URL, "qwerty1234")
	checkFeature(t, gb3, "foo", nil)
	knownWarnings(t, 1)
	gb3.LoadFeatures(nil)
	checkFeature(t, gb3, "foo", "initial")

	env.checkCalls(t, 1)

	// Old instances should also get cached value.
	checkFeature(t, gb, "foo", "initial")

	// Refreshing while cache is fresh should not cause a new network
	// request.
	gb.RefreshFeatures(nil)
	env.checkCalls(t, 1)

	// Wait a bit for cache to become stale and refresh again.
	time.Sleep(100 * time.Millisecond)
	gb.RefreshFeatures(nil)
	env.checkCalls(t, 2)

	// The instance being updated should get the new value.
	checkFeature(t, gb, "foo", "changed")

	// The instance with auto-refresh should now have the new value.
	checkFeature(t, gb2, "foo", "changed")

	// The instance without auto-refresh should continue to have the old
	// value.
	checkFeature(t, gb3, "foo", "initial")

	// New instances should get the new value
	gb4 := makeGB(env.server.URL, "qwerty1234")
	checkFeature(t, gb4, "foo", nil)
	knownWarnings(t, 1)
	gb4.LoadFeatures(nil)
	checkFeature(t, gb4, "foo", "changed")

	env.checkCalls(t, 2)
}

func TestRepoUpdatesFeaturesBasedOnSSE1(t *testing.T) {
	env := setup(true)
	defer cache.clear()
	defer checkLogs(t)
	defer env.close()

	cache.clear()

	gb := makeGB(env.server.URL, "qwerty1234")

	// Load features and check API calls.
	gb.LoadFeatures(&FeatureRepoOptions{AutoRefresh: true})
	env.checkCalls(t, 1)

	// Check feature before SSE message.
	checkFeature(t, gb, "foo", "initial")

	// Trigger mock SSE send.
	featuresJson := `{"features": {"foo": {"defaultValue": "changed"}}}`
	env.sseServer.Publish("features", &sse.Event{Data: []byte(featuresJson)})

	// Wait a little...
	time.Sleep(20 * time.Millisecond)

	// Check feature after SSE message.
	checkFeature(t, gb, "foo", "changed")
	env.checkCalls(t, 1)
}

func TestRepoUpdatesFeaturesBasedOnSSE2(t *testing.T) {
	env := setup(true)
	defer cache.clear()
	defer checkLogs(t)
	defer env.close()

	cache.clear()

	gb := makeGB(env.server.URL, "qwerty1234")
	gb2 := makeGB(env.server.URL, "qwerty1234")

	// Load features and check API calls.
	gb.LoadFeatures(nil)
	gb2.LoadFeatures(&FeatureRepoOptions{AutoRefresh: true})
	env.checkCalls(t, 1)

	// Check feature before SSE message.
	checkFeature(t, gb, "foo", "initial")
	checkFeature(t, gb2, "foo", "initial")

	// Trigger mock SSE send.
	featuresJson := `{"features": {"foo": {"defaultValue": "changed"}}}`
	env.sseServer.Publish("features", &sse.Event{Data: []byte(featuresJson)})

	// Wait a little...
	time.Sleep(20 * time.Millisecond)

	// Check feature after SSE message.
	checkFeature(t, gb, "foo", "initial")
	checkFeature(t, gb2, "foo", "changed")
	env.checkCalls(t, 1)
}

func TestRepoExposesAReadyFlag(t *testing.T) {
	env := setup(false)
	defer cache.clear()
	defer checkLogs(t)
	defer env.close()

	cache.clear()
	*env.featureValue = "api"

	gb := makeGB(env.server.URL, "qwerty1234")

	if gb.Ready() {
		t.Error("expected ready flag to be false")
	}
	gb.LoadFeatures(nil)
	env.checkCalls(t, 1)
	if !gb.Ready() {
		t.Error("expected ready flag to be true")
	}

	gb2 := makeGB(env.server.URL, "qwerty1234")
	if gb2.Ready() {
		t.Error("expected ready flag to be false")
	}
	gb2.WithFeatures(FeatureMap{"foo": &Feature{DefaultValue: "manual"}})
	if !gb2.Ready() {
		t.Error("expected ready flag to be false")
	}
}

func TestRepoHandlesBrokenFetchResponses(t *testing.T) {
	env := setup(false)
	defer cache.clear()
	defer checkLogs(t)
	defer env.close()

	cache.clear()
	env.nullFeatures = true

	gb := makeGB(env.server.URL, "qwerty1234")
	checkReady(t, gb, false)
	gb.LoadFeatures(nil)

	// Attempts network request, logs the error.
	env.checkCalls(t, 1)
	knownErrors(t, "Error fetching features")

	// Ready state changes to true
	checkReady(t, gb, true)
	checkEmptyFeatures(t, gb)

	// Logs the error, doesn't cache result.
	gb.RefreshFeatures(nil)
	checkEmptyFeatures(t, gb)
	env.checkCalls(t, 2)
	knownErrors(t, "Error fetching features")

	checkLogs(t)
}

func TestRepoHandlesSuperLongAPIRequests(t *testing.T) {
	env := setupWithDelay(false, 100*time.Millisecond)
	defer cache.clear()
	defer checkLogs(t)
	defer env.close()

	cache.clear()
	*env.featureValue = "api"

	gb := makeGB(env.server.URL, "qwerty1234")
	checkReady(t, gb, false)

	// Doesn't throw errors.
	gb.LoadFeatures(&FeatureRepoOptions{Timeout: 20 * time.Millisecond})
	env.checkCalls(t, 1)
	checkLogs(t)

	// Ready state remains false.
	checkReady(t, gb, false)
	checkEmptyFeatures(t, gb)

	// After fetch finished in the background, refreshing should
	// actually finish in time.
	time.Sleep(100 * time.Millisecond)
	gb.RefreshFeatures(&FeatureRepoOptions{Timeout: 20 * time.Millisecond})
	env.checkCalls(t, 1)
	checkReady(t, gb, true)
	checkFeature(t, gb, "foo", "api")
}

func TestRepoHandlesSSEErrors(t *testing.T) {
	env := setup(true)
	defer cache.clear()
	defer checkLogs(t)
	defer env.close()

	cache.clear()

	gb := makeGB(env.server.URL, "qwerty1234")

	gb.LoadFeatures(&FeatureRepoOptions{AutoRefresh: true})
	env.checkCalls(t, 1)
	checkFeature(t, gb, "foo", "initial")

	// Simulate SSE data.
	env.sseServer.Publish("features", &sse.Event{Data: []byte("broken(response")})

	// After SSE fired, should log an error and feature value should
	// remain the same.
	time.Sleep(20 * time.Millisecond)
	env.checkCalls(t, 1)
	checkFeature(t, gb, "foo", "initial")
	knownErrors(t, "SSE error")

	cache.clear()
}

// TODO: BIGGER TEST FOR SSE ERROR HANDLING

// func TestRepoDoesntDoBackgroundSyncWhenDisabled(t *testing.T) {

// }

// func TestRepoDecryptsFeatures(t *testing.T) {

// }
