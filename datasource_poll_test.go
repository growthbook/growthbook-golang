package growthbook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPollingDataSource(t *testing.T) {
	ctx := context.TODO()
	featuresJSON := []byte(`{
      "features": {
        "foo": {
          "defaultValue": "api"
        }
      },
      "experiments": [],
      "dateUpdated": "2000-05-01T00:00:12Z"
    }`)
	features := FeatureMap{"foo": &Feature{DefaultValue: "api"}}

	t.Run("Update client data from valid server response", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
		logger, _ := testLogger(slog.LevelError, t)
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(100*time.Millisecond),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		require.Nil(t, err)
		require.Equal(t, features, client.Features())
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Closing client stops data loading", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
		logger, _ := testLogger(slog.LevelInfo, t)
		defer ts.http.Close()
		client, _ := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(10*time.Millisecond),
		)
		client.EnsureLoaded(ctx)
		client.Close()
		require.True(t, ts.count.Load() > 0)
		ts.count.Store(0)
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, int32(0), ts.count.Load())
	})

	t.Run("EnsureLoaded returns error on invalid server response", func(t *testing.T) {
		ts := startServer(http.StatusNotFound, []byte(""))
		logger, _ := testLogger(slog.LevelError, t)
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(100*time.Millisecond),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		require.Error(t, fmt.Errorf("Error loading from server, code: %d,", http.StatusNotFound), err)
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Use etags for requests if present", func(t *testing.T) {
		ts := startEtagServer(featuresJSON)
		logger, _ := testLogger(slog.LevelError, t)
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(10*time.Millisecond),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		require.Nil(t, err)
		require.Equal(t, features, client.Features())
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, features, client.Features())
		require.True(t, ts.count.Load() > 2)
		require.Equal(t, ts.count.Load()-1, ts.etagCount.Load())
		// Without Close the poller outlives the test and keeps hitting this
		// port, which a later test's server may be assigned.
		require.Nil(t, client.Close())
	})

	t.Run("Concurrent Close calls during active polling - data race test", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
		logger, _ := testLogger(slog.LevelError, t)
		defer ts.http.Close()

		// Use a test context with timeout
		testCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, err := NewClient(testCtx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(5*time.Millisecond),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(testCtx)
		require.Nil(t, err)

		// Allow polling to run for a bit
		time.Sleep(50 * time.Millisecond)

		// Launch multiple concurrent Close() calls
		var wg sync.WaitGroup
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				client.Close()
			}()
		}
		wg.Wait()

		// Should complete without data race
	})

	t.Run("Close immediately after Start - data race test", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
		logger, _ := testLogger(slog.LevelError, t)
		defer ts.http.Close()

		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(1*time.Millisecond),
		)
		require.Nil(t, err)

		// Close should be safe while polling is happening
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Etag concurrent access during polling - data race test", func(t *testing.T) {
		ts := startEtagServer(featuresJSON)
		logger, _ := testLogger(slog.LevelError, t)
		defer ts.http.Close()

		// Use a test context with timeout
		testCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, err := NewClient(testCtx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(5*time.Millisecond),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(testCtx)
		require.Nil(t, err)

		// Allow multiple polls to occur and update etag
		time.Sleep(50 * time.Millisecond)

		// Verify etag was properly managed
		require.True(t, ts.count.Load() > 3, "Expected multiple polling requests")
		require.True(t, ts.etagCount.Load() > 0, "Expected etag requests")

		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("304 fires NotModified", func(t *testing.T) {
		ts := startSequenceServer(
			sequenceResponse{http.StatusOK, featuresJSON},
			sequenceResponse{http.StatusNotModified, nil},
		)
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelError, t)
		var c refreshCollector
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey(sequenceClientKey),
			WithFeaturesRefreshHandler(c.handler()),
			WithPollDataSource(10*time.Millisecond),
		)
		require.Nil(t, err)
		defer client.Close()
		require.Nil(t, client.EnsureLoaded(ctx))
		events := ts.waitForAttempts(t, &c, 2)

		stored := time.Date(2000, 5, 1, 0, 0, 12, 0, time.UTC)
		requireEvents(t, events,
			RefreshResult{Source: RefreshSourcePoll, Updated: true, DateUpdated: stored},
			RefreshResult{Source: RefreshSourcePoll, NotModified: true, DateUpdated: stored},
		)
	})

	// A 200 payload older than the stored data is refused by applyApiResponse.
	// The event must report what the client kept, not the timestamp it refused,
	// and must not claim an update.
	t.Run("stale response fires NotModified with the stored timestamp", func(t *testing.T) {
		staleJSON := []byte(`{
      "features": {
        "foo": {
          "defaultValue": "stale"
        }
      },
      "dateUpdated": "1999-01-01T00:00:00Z"
    }`)
		ts := startSequenceServer(
			sequenceResponse{http.StatusOK, featuresJSON},
			sequenceResponse{http.StatusOK, staleJSON},
			sequenceResponse{http.StatusOK, staleJSON},
		)
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelError, t)
		var c refreshCollector
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey(sequenceClientKey),
			WithFeaturesRefreshHandler(c.handler()),
			WithPollDataSource(10*time.Millisecond),
		)
		require.Nil(t, err)
		defer client.Close()
		require.Nil(t, client.EnsureLoaded(ctx))
		events := ts.waitForAttempts(t, &c, 3)

		stored := time.Date(2000, 5, 1, 0, 0, 12, 0, time.UTC)
		require.Equal(t, features, client.Features(), "a stale payload must not be applied")
		requireEvents(t, events,
			RefreshResult{Source: RefreshSourcePoll, Updated: true, DateUpdated: stored},
			RefreshResult{Source: RefreshSourcePoll, NotModified: true, DateUpdated: stored},
			RefreshResult{Source: RefreshSourcePoll, NotModified: true, DateUpdated: stored},
		)
	})

	t.Run("200 fires Updated", func(t *testing.T) {
		features2JSON := []byte(`{"features": {"foo": {"defaultValue": "poll"}}, "dateUpdated": "2000-05-02T00:00:12Z"}`)
		ts := startSequenceServer(
			sequenceResponse{http.StatusOK, featuresJSON},
			sequenceResponse{http.StatusOK, features2JSON},
		)
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelError, t)
		var c refreshCollector
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey(sequenceClientKey),
			WithFeaturesRefreshHandler(c.handler()),
			WithPollDataSource(10*time.Millisecond),
		)
		require.Nil(t, err)
		defer client.Close()
		require.Nil(t, client.EnsureLoaded(ctx))
		events := ts.waitForAttempts(t, &c, 2)

		requireEvents(t, events,
			RefreshResult{Source: RefreshSourcePoll, Updated: true, DateUpdated: time.Date(2000, 5, 1, 0, 0, 12, 0, time.UTC)},
			RefreshResult{Source: RefreshSourcePoll, Updated: true, DateUpdated: time.Date(2000, 5, 2, 0, 0, 12, 0, time.UTC)},
		)
	})

	// A failed initial load aborts Start, so polling never begins: the single
	// attempt must produce exactly one Error event.
	t.Run("error fires Error", func(t *testing.T) {
		ts := startServer(http.StatusNotFound, []byte(""))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelError, t)
		var c refreshCollector
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithFeaturesRefreshHandler(c.handler()),
			WithPollDataSource(10*time.Millisecond),
		)
		require.Nil(t, err)
		require.NotNil(t, client.EnsureLoaded(ctx))

		requireEvents(t, c.all(),
			RefreshResult{Source: RefreshSourcePoll, Error: errors.New("any")},
		)
	})

	t.Run("panicking handler does not break polling", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
		defer ts.http.Close()
		panicHandler := func(ctx context.Context, r RefreshResult) { panic("boom") }
		logger, _ := testLogger(slog.LevelError, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithFeaturesRefreshHandler(panicHandler),
			WithPollDataSource(10*time.Millisecond),
		)
		require.Nil(t, err)
		require.Nil(t, client.EnsureLoaded(ctx))
		// The handler panics on every attempt; polling must keep going.
		require.Eventually(t, func() bool { return ts.count.Load() >= 3 },
			2*time.Second, time.Millisecond, "polling stopped after a handler panic")
		require.Nil(t, client.Close())
	})
}

// refreshCollector safely gathers RefreshResult events produced by the
// datasource goroutine so the test goroutine can inspect them without a race.
type refreshCollector struct {
	mu      sync.Mutex
	results []RefreshResult
}

// handler returns a FeaturesRefreshHandler that appends every event under lock.
func (c *refreshCollector) handler() FeaturesRefreshHandler {
	return func(ctx context.Context, r RefreshResult) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.results = append(c.results, r)
	}
}

// all returns a copy of the collected events, so the caller can iterate
// without holding the lock (and without racing the datasource goroutine).
func (c *refreshCollector) all() []RefreshResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]RefreshResult(nil), c.results...)
}

// waitFor blocks until pred matches a collected event, failing the test after
// a timeout, and returns every event collected up to that point.
func (c *refreshCollector) waitFor(t *testing.T, pred func(RefreshResult) bool) []RefreshResult {
	t.Helper()
	var events []RefreshResult
	require.Eventually(t, func() bool {
		events = c.all()
		for _, r := range events {
			if pred(r) {
				return true
			}
		}
		return false
	}, 2*time.Second, time.Millisecond, "the expected refresh event never arrived")
	return events
}

// requireEvents asserts the exact sequence of refresh events, field by field.
// Error is compared by presence only; DateUpdated with Equal, since time.Time
// values must not be compared with ==.
func requireEvents(t *testing.T, got []RefreshResult, want ...RefreshResult) {
	t.Helper()
	require.Len(t, got, len(want), "expected exactly one event per attempt, got %+v", got)
	for i := range want {
		require.Equal(t, want[i].Source, got[i].Source, "event %d: Source", i)
		require.Equal(t, want[i].Updated, got[i].Updated, "event %d: Updated", i)
		require.Equal(t, want[i].NotModified, got[i].NotModified, "event %d: NotModified", i)
		require.Equal(t, want[i].Error != nil, got[i].Error != nil, "event %d: Error = %v", i, got[i].Error)
		require.True(t, want[i].DateUpdated.Equal(got[i].DateUpdated),
			"event %d: DateUpdated want %v, got %v", i, want[i].DateUpdated, got[i].DateUpdated)
	}
}

type testServer struct {
	http      *httptest.Server
	count     atomic.Int32
	etagCount atomic.Int32
}

func startServer(code int, response []byte) *testServer {
	var ts testServer
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.count.Add(1)
		w.WriteHeader(code)
		_, _ = w.Write(response)
	}))
	return &ts
}

type sequenceResponse struct {
	status int
	body   []byte
}

// sequenceClientKey is the only client key startSequenceServer answers.
const sequenceClientKey = "sequencekey"

// startSequenceServer answers the i-th request with responses[i] and holds
// every later request open until the client cancels it, so the test fixes the
// number of refresh attempts instead of racing a sleep against the poller.
// Requests for any other client key are rejected without being counted: a
// poller leaked by another test may hit a reused port, and must neither consume
// a response nor be held open (which would block the server's Close).
func startSequenceServer(responses ...sequenceResponse) *testServer {
	var ts testServer
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/features/"+sequenceClientKey {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		i := int(ts.count.Add(1)) - 1
		if i >= len(responses) {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(responses[i].status)
		_, _ = w.Write(responses[i].body)
	}))
	return &ts
}

// waitForAttempts waits until the poller has issued the request after the
// n-th one and returns the events collected so far. Polling is sequential and
// the handler runs before the next request, so once request n+1 arrives every
// event of attempts 1..n has been delivered - including any duplicate - and the
// held request n+1 cannot have produced one yet.
func (ts *testServer) waitForAttempts(t *testing.T, c *refreshCollector, n int) []RefreshResult {
	t.Helper()
	require.Eventually(t, func() bool { return int(ts.count.Load()) > n },
		2*time.Second, time.Millisecond, "expected %d poll attempts", n+1)
	return c.all()
}

func startEtagServer(response []byte) *testServer {
	var ts testServer
	etag := `W/"SOME_ETAG_VALUE"`
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.count.Add(1)
		if r.Header.Get("If-None-Match") == etag {
			ts.etagCount.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("etag", etag)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(response)
	}))
	return &ts
}

func TestPollingAppliesPartialResponses(t *testing.T) {
	ctx := context.TODO()

	t.Run("a bandit-only response updates definitions and preserves features", func(t *testing.T) {
		ts := startServer(http.StatusOK, []byte(`{
			"contextualBandits": {
				"cb-1": {"contexts": [{"leafId": 10, "condition": {}, "weights": [1, 0]}]}
			},
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelError, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithJsonFeatures(`{"bandit-flag": {"defaultValue": "default", "rules": [{
				"key": "bandit-exp", "coverage": 1, "contextualBanditRef": "cb-1",
				"contextualVariations": ["a", "b"], "weights": [0.5, 0.5]
			}]}}`),
			WithAttributes(Attributes{"id": "u1"}),
			WithPollDataSource(100*time.Millisecond),
		)
		require.Nil(t, err)
		defer client.Close()
		require.Nil(t, client.EnsureLoaded(ctx))

		res := client.EvalFeature(ctx, "bandit-flag")
		require.True(t, res.InExperiment(), "features must survive a bandit-only poll response")
		require.Equal(t, 10, *res.ExperimentResult.LeafId, "the polled definitions must apply")
	})

	t.Run("a 304 response changes nothing", func(t *testing.T) {
		ts := startServer(http.StatusNotModified, nil)
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelError, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithJsonFeatures(`{"foo": {"defaultValue": "kept"}}`),
			WithPollDataSource(100*time.Millisecond),
		)
		require.Nil(t, err)
		defer client.Close()
		require.Nil(t, client.EnsureLoaded(ctx))
		require.Equal(t, "kept", client.EvalFeature(ctx, "foo").Value)
	})
}
