package growthbook

import (
	"context"
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
		ts := startEtagServer(featuresJSON)
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
		require.Nil(t, client.EnsureLoaded(ctx))
		time.Sleep(100 * time.Millisecond)
		require.Nil(t, client.Close())
		var got bool
		for _, r := range c.all() {
			if r.NotModified {
				got = true
			}
		}
		require.True(t, got, "expected at least one NotModified event")
	})

	t.Run("200 fires Updated", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
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
		require.Nil(t, client.EnsureLoaded(ctx))
		time.Sleep(100 * time.Millisecond)
		require.Nil(t, client.Close())
		var got bool
		for _, r := range c.all() {
			if r.Updated {
				got = true
			}
		}
		require.True(t, got, "expected at least one Updated event")
	})

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
		time.Sleep(100 * time.Millisecond)
		require.Nil(t, client.Close())
		var got bool
		for _, r := range c.all() {
			if r.Error != nil {
				got = true
			}
		}
		require.True(t, got, "expected at least one Error event")
	})

	t.Run("panicking handler does not break polling", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
		defer ts.http.Close()
		panicHandler := func(ctx context.Context, r RefreshResult) { panic("boom") }
		logger, _ := testLogger(slog.LevelError, t)
		client, _ := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithFeaturesRefreshHandler(panicHandler),
			WithPollDataSource(10*time.Millisecond),
		)

		require.Nil(t, client.EnsureLoaded(ctx))
		time.Sleep(50 * time.Millisecond)
		n := ts.count.Load()
		time.Sleep(50 * time.Millisecond)
		require.Greater(t, ts.count.Load(), n)
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

// has reports whether any collected event matches pred.
func (c *refreshCollector) has(pred func(RefreshResult) bool) bool {
	for _, r := range c.all() {
		if pred(r) {
			return true
		}
	}
	return false
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

// startVersionedServer returns each response in order on successive requests;
// once the list is exhausted it keeps returning the last one. Useful for
// exercising ordered payloads (e.g. a fresh response followed by a stale one).
func startVersionedServer(responses ...[]byte) *testServer {
	var ts testServer
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(ts.count.Add(1)) - 1
		if i >= len(responses) {
			i = len(responses) - 1
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responses[i])
	}))
	return &ts
}
