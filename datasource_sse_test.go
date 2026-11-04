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

func TestSseDataSource(t *testing.T) {
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

	features2JSON := `{"features": { "foo": { "defaultValue": "SSE" } }, "experiments": [], "dateUpdated": "2000-05-02T00:00:12Z" }`
	features2 := FeatureMap{"foo": &Feature{DefaultValue: "SSE"}}

	t.Run("Update client data from sse data", func(t *testing.T) {
		ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 0))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelWarn, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		require.Equal(t, features, client.Features())
		require.Nil(t, err)
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, features2, client.Features())
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Reconnect to server on connection break", func(t *testing.T) {
		ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 3))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelWarn, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		_ = client.EnsureLoaded(ctx)
		time.Sleep(100 * time.Millisecond)
		require.Greater(t, ts.ssecount.Load(), int32(1))
		require.Equal(t, features2, client.Features())
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Don't reconnect after closing client", func(t *testing.T) {
		ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 3))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelWarn, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		_ = client.EnsureLoaded(ctx)
		client.Close()
		old := ts.ssecount.Load()
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, old, ts.ssecount.Load())
	})

	t.Run("Concurrent Close calls during active SSE connection - data race test", func(t *testing.T) {
		ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 0))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelWarn, t)

		// Use a test context with timeout
		testCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, err := NewClient(testCtx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(testCtx)
		require.Nil(t, err)

		// Allow SSE connection to establish
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
		ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 0))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelWarn, t)

		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithSseDataSource(),
		)
		require.Nil(t, err)

		// Close should be safe while connection is active
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Multiple rapid Start/Close cycles - data race test", func(t *testing.T) {
		// Test that multiple clients can be created and closed without races
		for cycle := 0; cycle < 3; cycle++ {
			ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 0))
			logger, _ := testLogger(slog.LevelWarn, t)

			client, err := NewClient(ctx,
				WithLogger(logger),
				WithHttpClient(ts.http.Client()),
				WithApiHost(ts.http.URL),
				WithClientKey("somekey"),
				WithSseDataSource(),
			)
			require.Nil(t, err)

			// Close immediately
			err = client.Close()
			require.Nil(t, err)

			ts.http.Close()
		}
	})

	// Each streamed test ends the stream with sentinelEvent: events are
	// dispatched in order on one goroutine, so once the sentinel's event
	// arrives every earlier event - including any duplicate - has been
	// delivered, and the exact sequence can be asserted. The sentinel carries
	// only an (empty) savedGroups section, so it never touches features.
	sentinelEvent := `{"savedGroups": {}, "dateUpdated": "2000-05-09T00:00:12Z"}`
	sentinelDate := time.Date(2000, 5, 9, 0, 0, 12, 0, time.UTC)
	stored := time.Date(2000, 5, 1, 0, 0, 12, 0, time.UTC)
	isSentinel := func(r RefreshResult) bool { return r.Updated && r.DateUpdated.Equal(sentinelDate) }

	t.Run("Fires Updated events for initial load and streamed update", func(t *testing.T) {
		ts := startSseServer(featuresJSON, sseEvents(features2JSON, sentinelEvent))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelWarn, t)
		var c refreshCollector
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithFeaturesRefreshHandler(c.handler()),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		defer client.Close()
		require.Nil(t, client.EnsureLoaded(ctx))
		events := c.waitFor(t, isSentinel)

		require.Equal(t, features2, client.Features())
		requireEvents(t, events,
			RefreshResult{Source: RefreshSourceSSE, Updated: true, DateUpdated: stored},
			RefreshResult{Source: RefreshSourceSSE, Updated: true, DateUpdated: time.Date(2000, 5, 2, 0, 0, 12, 0, time.UTC)},
			RefreshResult{Source: RefreshSourceSSE, Updated: true, DateUpdated: sentinelDate},
		)
	})

	// A streamed event that omits "features" carries nothing to apply. It must
	// leave the loaded features in place (issue #70) and report NotModified,
	// not Updated - the stream path reaches applyApiResponse directly, so this
	// is the path where a payload-shaped no-op used to look like a refresh.
	t.Run("Streamed event with no payload keeps features and reports NotModified", func(t *testing.T) {
		emptyEvent := `{"dateUpdated": "2000-05-03T00:00:12Z"}`
		ts := startSseServer(featuresJSON, sseEvents(emptyEvent, sentinelEvent))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelError, t)
		var c refreshCollector
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithFeaturesRefreshHandler(c.handler()),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		defer client.Close()
		require.Nil(t, client.EnsureLoaded(ctx))
		events := c.waitFor(t, isSentinel)

		require.Equal(t, features, client.Features(), "an empty event must not wipe the loaded features")
		requireEvents(t, events,
			RefreshResult{Source: RefreshSourceSSE, Updated: true, DateUpdated: stored},
			RefreshResult{Source: RefreshSourceSSE, NotModified: true, DateUpdated: time.Date(2000, 5, 3, 0, 0, 12, 0, time.UTC)},
			RefreshResult{Source: RefreshSourceSSE, Updated: true, DateUpdated: sentinelDate},
		)
	})

	// A streamed event older than the stored data is refused. The handler must
	// report the timestamp the client kept, not the one it rejected.
	t.Run("Stale streamed event reports the stored timestamp", func(t *testing.T) {
		staleEvent := `{"features": {"foo": {"defaultValue": "stale"}}, "dateUpdated": "1999-01-01T00:00:00Z"}`
		ts := startSseServer(featuresJSON, sseEvents(staleEvent, sentinelEvent))
		defer ts.http.Close()
		logger, _ := testLogger(slog.LevelError, t)
		var c refreshCollector
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithFeaturesRefreshHandler(c.handler()),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		defer client.Close()
		require.Nil(t, client.EnsureLoaded(ctx))
		events := c.waitFor(t, isSentinel)

		require.Equal(t, features, client.Features(), "a stale event must not be applied")
		requireEvents(t, events,
			RefreshResult{Source: RefreshSourceSSE, Updated: true, DateUpdated: stored},
			RefreshResult{Source: RefreshSourceSSE, NotModified: true, DateUpdated: stored},
			RefreshResult{Source: RefreshSourceSSE, Updated: true, DateUpdated: sentinelDate},
		)
	})

	// A failed initial load aborts Start before the stream connects: the single
	// attempt must produce exactly one Error event.
	t.Run("Fires Error event when initial load fails", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()
		logger, _ := testLogger(slog.LevelError, t)
		var c refreshCollector
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(ts.Client()),
			WithApiHost(ts.URL),
			WithClientKey("somekey"),
			WithFeaturesRefreshHandler(c.handler()),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		require.Error(t, client.EnsureLoaded(ctx))

		requireEvents(t, c.all(),
			RefreshResult{Source: RefreshSourceSSE, Error: errors.New("any")},
		)
	})
}

func TestSseDataSourceRetryInterval(t *testing.T) {
	ctx := context.TODO()

	t.Run("default max retry interval", func(t *testing.T) {
		client, err := NewClient(ctx, WithSseDataSource())
		require.Nil(t, err)
		ds, ok := client.data.dataSource.(*SseDataSource)
		require.True(t, ok)
		require.Equal(t, defaultMaxRetryInterval, ds.maxRetryInterval)
	})

	t.Run("custom max retry interval", func(t *testing.T) {
		client, err := NewClient(ctx, WithSseDataSource(WithSseMaxRetryInterval(time.Minute)))
		require.Nil(t, err)
		ds, ok := client.data.dataSource.(*SseDataSource)
		require.True(t, ok)
		require.Equal(t, time.Minute, ds.maxRetryInterval)
	})

	t.Run("non-positive max retry interval keeps default", func(t *testing.T) {
		client, err := NewClient(ctx, WithSseDataSource(WithSseMaxRetryInterval(0)))
		require.Nil(t, err)
		ds, ok := client.data.dataSource.(*SseDataSource)
		require.True(t, ok)
		require.Equal(t, defaultMaxRetryInterval, ds.maxRetryInterval)
	})

	t.Run("nil option is ignored", func(t *testing.T) {
		client, err := NewClient(ctx, WithSseDataSource(nil))
		require.Nil(t, err)
		ds, ok := client.data.dataSource.(*SseDataSource)
		require.True(t, ok)
		require.Equal(t, defaultMaxRetryInterval, ds.maxRetryInterval)
	})
}

type sseTestServer struct {
	http     *httptest.Server
	ssecount atomic.Int32
	apicount atomic.Int32
}

type sseResponseGen func(context.Context, http.ResponseWriter)

func startSseServer(apiResponse []byte, sseResponseGen sseResponseGen) *sseTestServer {
	var ts sseTestServer
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features/somekey":
			w.Header().Add("x-sse-support", "enabled")
			w.WriteHeader(http.StatusOK)
			w.Write(apiResponse)
			ts.apicount.Add(1)
			return
		case "/sub/somekey":
			ts.ssecount.Add(1)
			sseResponseGen(r.Context(), w)
		}
	}))
	return &ts
}

// sseEvents streams each payload once as a "features" event, in order, then
// holds the stream open until the client disconnects, so a test controls
// exactly which events the client receives.
func sseEvents(payloads ...string) sseResponseGen {
	return func(ctx context.Context, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)
		for i, p := range payloads {
			fmt.Fprintf(w, "id: %d\nevent: features\ndata: %s\n\n", i+1, p)
		}
		flusher.Flush()
		<-ctx.Done()
	}
}

func sseResponse(response string, delay time.Duration, lim int) sseResponseGen {
	stream := []string{
		"retry: 10\n\n",
		"data:\n\n",
		fmt.Sprintf("id: 1\nevent: features\ndata: %s\n\n", response),
		"data:\n\n",
	}

	return func(ctx context.Context, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		ticker := time.NewTicker(delay)
		defer ticker.Stop()
		flusher := w.(http.Flusher)
		flusher.Flush()
		for count := 0; lim == 0 || count < lim; count++ {
			select {
			case <-ticker.C:
				if len(stream) > count {
					w.Write([]byte(stream[count]))
				} else {
					w.Write([]byte("data:\n\n"))
				}
				flusher.Flush()
				count++
			case <-ctx.Done():
				return
			}
		}
	}
}
