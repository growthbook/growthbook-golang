package growthbook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ian-ross/sse/v2"
)

type env struct {
	sync.RWMutex
	server       *httptest.Server
	sseServer    *sse.Server
	fetchFails   bool
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

func setupEncrypted(features string, provideSSE bool) *env {
	return setupWithDelay(provideSSE, 50*time.Millisecond, features)
}

func setup(provideSSE bool) *env {
	return setupWithDelay(provideSSE, 50*time.Millisecond, "")
}

func setupWithDelay(provideSSE bool, delay time.Duration, encryptedFeatures string) *env {
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

		if env.fetchFails {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("fetch failed"))
		} else {
			features := FeatureMap{}
			if encryptedFeatures == "" {
				features = FeatureMap{"foo": {DefaultValue: *env.featureValue}}
			}
			response := &FeatureAPIResponse{
				Features:          features,
				DateUpdated:       time.Now(),
				EncryptedFeatures: encryptedFeatures,
			}

			if provideSSE {
				w.Header().Set("X-SSE-Support", "enabled")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}
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

func makeGB(apiHost string, clientKey string, ttl time.Duration) *GrowthBook {
	context := NewContext().
		WithAPIHost(apiHost).
		WithClientKey(clientKey)
	if ttl != 0 {
		context = context.WithCacheTTL(ttl)
	}
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
		bad := false
		for _, e := range testLog.errors {
			if !strings.Contains(e, "SSE event stream disconnected") {
				bad = true
			}
		}
		if bad {
			t.Errorf("test log has warnings: %s", testLog.allWarnings())
		}
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

func knownSSEErrors(t *testing.T) func() {
	return func() {
		for _, msg := range testLog.errors {
			if !strings.HasPrefix(msg, "SSE error:") {
				t.Errorf("unexpected error in log: '%s'", msg)
			}
		}
	}
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
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()

	gb1 := makeGB(env.server.URL, "qwerty1234", 0)
	gb2 := makeGB(env.server.URL, "other", 0)
	gb3 := makeGB(env.server.URL, "qwerty1234", 0)

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
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()

	// Set cache TTL short so we can test expiry.
	gb := makeGB(env.server.URL, "qwerty1234", 100*time.Millisecond)
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
	gb2 := makeGB(env.server.URL, "qwerty1234", 100*time.Millisecond)
	checkFeature(t, gb2, "foo", nil)
	knownWarnings(t, 1)
	gb2.LoadFeatures(&FeatureRepoOptions{AutoRefresh: true})
	checkFeature(t, gb2, "foo", "initial")

	// Instance without autoRefresh.
	gb3 := makeGB(env.server.URL, "qwerty1234", 100*time.Millisecond)
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
	gb4 := makeGB(env.server.URL, "qwerty1234", 100*time.Millisecond)
	checkFeature(t, gb4, "foo", nil)
	knownWarnings(t, 1)
	gb4.LoadFeatures(nil)
	checkFeature(t, gb4, "foo", "changed")

	env.checkCalls(t, 2)
}

func TestRepoUpdatesFeaturesBasedOnSSE1(t *testing.T) {
	env := setup(true)
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()

	gb := makeGB(env.server.URL, "qwerty1234", 0)

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
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()

	gb := makeGB(env.server.URL, "qwerty1234", 0)
	gb2 := makeGB(env.server.URL, "qwerty1234", 0)

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
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()
	*env.featureValue = "api"

	gb := makeGB(env.server.URL, "qwerty1234", 0)

	if gb.Ready() {
		t.Error("expected ready flag to be false")
	}
	gb.LoadFeatures(nil)
	env.checkCalls(t, 1)
	if !gb.Ready() {
		t.Error("expected ready flag to be true")
	}

	gb2 := makeGB(env.server.URL, "qwerty1234", 0)
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
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()
	env.fetchFails = true

	gb := makeGB(env.server.URL, "qwerty1234", 0)
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
	env := setupWithDelay(false, 100*time.Millisecond, "")
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()
	*env.featureValue = "api"

	gb := makeGB(env.server.URL, "qwerty1234", 0)
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
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()

	gb := makeGB(env.server.URL, "qwerty1234", 0)

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

	cache.Clear()
}

// This is a more complex test scenario for checking that parallel
// handling of auto-refresh, SSE updates and SSE errors works
// correctly together.

// Disabled for the moment...

// func TestRepoComplexSSEScenario(t *testing.T) {
// 	env := setup(true)
// 	defer cache.Clear()
// 	// We're going to generate SSE errors here, but that's all we expect
// 	// to see.
// 	defer knownSSEErrors(t)
// 	defer env.close()

// 	cache.Clear()

// 	// Data recording for test goroutines.
// 	type record struct {
// 		result string
// 		t      time.Time
// 	}

// 	var wg sync.WaitGroup

// 	// Test function to run in a goroutine: evaluates features at
// 	// randomly spaced intervals, storing the results and the sample
// 	// times, until told to stop.
// 	tester := func(gb *GrowthBook, doneCh chan struct{}, vals *[]*record) {
// 		defer wg.Done()
// 		tick := time.NewTicker(time.Duration(100+rand.Intn(100)) * time.Millisecond)
// 		gb.LoadFeatures(&FeatureRepoOptions{AutoRefresh: true})
// 		for {
// 			select {
// 			case <-doneCh:
// 				return

// 			case <-tick.C:
// 				f, _ := gb.EvalFeature("foo").Value.(string)
// 				*vals = append(*vals, &record{f, time.Now()})
// 			}
// 		}
// 	}

// 	// Set up test goroutines, each with an independent GrowthBook
// 	// instance, cancellation channel and result storage.
// 	gbs := make([]*GrowthBook, 10)
// 	doneChs := make([]chan struct{}, 10)
// 	vals := make([][]*record, 10)
// 	wg.Add(10)
// 	for i := 0; i < 10; i++ {
// 		gbs[i] = makeGB(env.server.URL, "qwerty1234", 0)
// 		doneChs[i] = make(chan struct{})
// 		vals[i] = []*record{}
// 		go tester(gbs[i], doneChs[i], &vals[i])
// 	}

// 	// Command storage.
// 	type command struct {
// 		cmd int
// 		t   time.Time
// 	}
// 	commands := make([]command, 100)

// 	// Command loop: send SSE events at random intervals, with
// 	// approximately 10% failure rate (and always at least three
// 	// failures in a row, to trigger SSE client reconnection).
// 	bad := 0
// 	for i := 0; i < 100; i++ {
// 		ok := rand.Intn(100) < 90
// 		if ok && bad == 0 {
// 			featuresJson := fmt.Sprintf(
// 				`{"features": {"foo": {"defaultValue": "val%d"}}, "dateUpdated": "%s"}`,
// 				i+1, time.Now().Format(dateLayout))
// 			commands[i] = command{i + 1, time.Now()}
// 			env.sseServer.Publish("features", &sse.Event{Data: []byte(featuresJson)})
// 		} else {
// 			if bad == 0 {
// 				bad = 3
// 			} else {
// 				bad--
// 			}
// 			commands[i] = command{-(i + 1), time.Now()}
// 			env.sseServer.Publish("features", &sse.Event{Data: []byte("broken(bad")})
// 		}
// 		time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)
// 	}

// 	// Stop the test goroutines and zero their GrowthBook instances so
// 	// that finalizers will run and background SSE refresh will stop
// 	// too.
// 	for i := 0; i < 10; i++ {
// 		doneChs[i] <- struct{}{}
// 		gbs[i] = nil
// 	}
// 	wg.Wait()

// 	// Check the results from the test goroutines by finding the
// 	// relevant times in the command history. Allow some slack for small
// 	// time differences.
// 	errors := 0
// 	for i := 0; i < 10; i++ {
// 		for _, v := range vals[i] {
// 			if v.result == "initial" {
// 				continue
// 			}
// 			cmdidx, _ := sortFind(len(commands), func(i int) int {
// 				if v.t == commands[i].t {
// 					return 0
// 				}
// 				if v.t.After(commands[i].t) {
// 					return 1
// 				}
// 				return -1
// 			})

// 			cmdidx--
// 			expected := fmt.Sprintf("val%d", commands[cmdidx].cmd)

// 			beforeidx := cmdidx - 1
// 			for beforeidx > 0 && commands[beforeidx].cmd < 0 {
// 				beforeidx--
// 			}
// 			before := fmt.Sprintf("val%d", commands[beforeidx].cmd)

// 			afteridx := cmdidx + 1
// 			for afteridx < len(commands)-1 && commands[afteridx].cmd < 0 {
// 				afteridx++
// 			}
// 			after := ""
// 			if afteridx < len(commands) {
// 				after = fmt.Sprintf("val%d", commands[afteridx].cmd)
// 			}

// 			if v.result != expected && v.result != before && v.result != after {
// 				errors++
// 				t.Error("unexpected feature value")
// 				fmt.Println(v.result, expected, v.t, cmdidx, beforeidx, afteridx)
// 			}
// 		}
// 	}
// }

func TestRepoDoesntDoBackgroundSyncWhenDisabled(t *testing.T) {
	env := setup(true)
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()
	ConfigureCacheBackgroundSync(false)
	defer ConfigureCacheBackgroundSync(true)

	gb := makeGB(env.server.URL, "qwerty1234", 0)
	gb2 := makeGB(env.server.URL, "qwerty1234", 0)

	gb.LoadFeatures(nil)
	gb2.LoadFeatures(&FeatureRepoOptions{AutoRefresh: true})

	// Initial value from API.
	env.checkCalls(t, 1)
	checkFeature(t, gb, "foo", "initial")
	checkFeature(t, gb2, "foo", "initial")

	// Trigger mock SSE send.
	featuresJson := `{"features": {"foo": {"defaultValue": "changed"}}}`
	env.sseServer.Publish("features", &sse.Event{Data: []byte(featuresJson)})

	// SSE update is ignored.
	time.Sleep(100 * time.Millisecond)
	checkFeature(t, gb, "foo", "initial")
	checkFeature(t, gb2, "foo", "initial")
	env.checkCalls(t, 1)
}

func TestRepoDecryptsFeatures(t *testing.T) {
	encryptedFeatures := "vMSg2Bj/IurObDsWVmvkUg==.L6qtQkIzKDoE2Dix6IAKDcVel8PHUnzJ7JjmLjFZFQDqidRIoCxKmvxvUj2kTuHFTQ3/NJ3D6XhxhXXv2+dsXpw5woQf0eAgqrcxHrbtFORs18tRXRZza7zqgzwvcznx"

	env := setupEncrypted(encryptedFeatures, false)
	defer cache.Clear()
	defer checkLogs(t)
	defer env.close()

	cache.Clear()

	context := NewContext().
		WithAPIHost(env.server.URL).
		WithClientKey("qwerty1234").
		WithDecryptionKey("Ns04T5n9+59rl2x3SlNHtQ==")
	gb := New(context)

	gb.LoadFeatures(nil)

	env.checkCalls(t, 1)

	expectedJson := `{
    "testfeature1": {
      "defaultValue": true,
      "rules": [{"condition": { "id": "1234" }, "force": false}]
    }
  }`
	expected := ParseFeatureMap([]byte(expectedJson))
	actual := gb.Features()

	if !reflect.DeepEqual(actual, expected) {
		t.Error("unexpected features value: ", actual)
	}
}
