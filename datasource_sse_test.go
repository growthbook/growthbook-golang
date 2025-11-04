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
		err = client.EnsureLoaded(ctx)
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
		err = client.EnsureLoaded(ctx)
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

		// Race between Start completing and Close being called
		ds := client.data.dataSource
		errChan := make(chan error, 1)
		go func() {
			errChan <- ds.Start(testCtx)
		}()

		// Immediately call Close while Start is completing
		go func() {
			time.Sleep(1 * time.Millisecond)
			ds.Close()
		}()

		err = <-errChan
		require.Nil(t, err)
	})

	t.Run("Multiple rapid Start/Close cycles - data race test", func(t *testing.T) {
		for cycle := 0; cycle < 3; cycle++ {
			ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 0))
			logger, _ := testLogger(slog.LevelWarn, t)

			// Use a cancellable context for this cycle
			cycleCtx, cancel := context.WithCancel(ctx)

			client, err := NewClient(cycleCtx,
				WithLogger(logger),
				WithHttpClient(ts.http.Client()),
				WithApiHost(ts.http.URL),
				WithClientKey("somekey"),
				WithSseDataSource(),
			)
			require.Nil(t, err)

			// Start and immediately close
			ds := client.data.dataSource
			go ds.Start(cycleCtx)
			time.Sleep(5 * time.Millisecond)

			// Cancel the context to stop SSE connection
			cancel()

			// Wait for connections to gracefully close
			time.Sleep(50 * time.Millisecond)

			// Then close server
			ts.http.Close()
			time.Sleep(10 * time.Millisecond)
		}
		// Should complete all cycles without data race
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
