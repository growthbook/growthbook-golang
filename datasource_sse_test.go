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

// silentSseStream holds a successful connection open without ever sending an event, the way a real
// stream behaves while no flag changes. Nothing but a reload can fill the client data here.
func silentSseStream(connected *atomic.Int32) sseResponseGen {
	return func(ctx context.Context, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		connected.Add(1)

		<-ctx.Done()
	}
}

func TestSseRetryDelayGrowsTowardsTheCap(t *testing.T) {
	const max = 30 * time.Second

	require.Equal(t, 1*time.Second, sseRetryDelay(max, 1))
	require.Equal(t, 2*time.Second, sseRetryDelay(max, 2))
	require.Equal(t, 4*time.Second, sseRetryDelay(max, 3))
	require.Equal(t, 8*time.Second, sseRetryDelay(max, 4))
	require.Equal(t, 16*time.Second, sseRetryDelay(max, 5))

	require.Equal(t, max, sseRetryDelay(max, 6), "the wait settles at the cap")
	require.Equal(t, max, sseRetryDelay(max, 1000), "and stays there rather than overflowing")

	require.Equal(t, 500*time.Millisecond, sseRetryDelay(500*time.Millisecond, 1),
		"a cap below the first step is a ceiling, not a floor")
	require.Equal(t, 4*time.Second, sseRetryDelay(0, 3), "a nonpositive cap falls back to the default ceiling")
	require.Equal(t, defaultMaxRetryInterval, sseRetryDelay(0, 99), "and settles there")
	require.Equal(t, 1*time.Second, sseRetryDelay(max, 0), "a nonpositive attempt counts as the first")
}

func TestSseKeepsTheFeaturesOfAHostThatDoesNotServeTheStream(t *testing.T) {
	var apiCalls, connections atomic.Int32
	stream := silentSseStream(&connections)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features/somekey":
			apiCalls.Add(1)
			// No x-sse-support header, as a proxy that strips it would leave things.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"features":{"feature":{"defaultValue":1}}}`))
		case "/sub/somekey":
			stream(r.Context(), w)
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.URL),
		WithClientKey("somekey"),
		WithSseDataSource(WithSseMaxRetryInterval(20*time.Millisecond)),
	)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	require.Error(t, client.EnsureLoaded(ctx), "the host does not serve the stream, and the caller learns that")
	require.NotNil(t, client.EvalFeature(ctx, "feature").Value,
		"the payload was valid and has to survive the missing header")

	settled := apiCalls.Load()
	time.Sleep(200 * time.Millisecond)

	require.Equal(t, settled, apiCalls.Load(),
		"nothing was left to retry once the load delivered its features")
}

func TestSseStopsRetryingTheInitialLoadOnARejectedKey(t *testing.T) {
	var apiCalls, connections atomic.Int32
	stream := silentSseStream(&connections)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features/somekey":
			apiCalls.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		case "/sub/somekey":
			stream(r.Context(), w)
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.URL),
		WithClientKey("somekey"),
		WithSseDataSource(WithSseMaxRetryInterval(20*time.Millisecond)),
	)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	require.Error(t, client.EnsureLoaded(ctx))

	time.Sleep(200 * time.Millisecond)
	settled := apiCalls.Load()
	time.Sleep(300 * time.Millisecond)

	require.Equal(t, settled, apiCalls.Load(),
		"a rejected key does not become valid by asking again - the retry has to give up")
}

func TestSseRetriesTheInitialLoadWhileTheStreamStaysSilent(t *testing.T) {
	features := `{"feature":{"defaultValue":1}}`

	var apiCalls, connections atomic.Int32
	stream := silentSseStream(&connections)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features/somekey":
			if apiCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Add("x-sse-support", "enabled")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"features":%s}`, features)))
		case "/sub/somekey":
			stream(r.Context(), w)
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.URL),
		WithClientKey("somekey"),
		WithSseDataSource(WithSseMaxRetryInterval(20*time.Millisecond)),
	)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	require.Error(t, client.EnsureLoaded(ctx), "the first load failed, and the caller learns that here")

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if client.EvalFeature(ctx, "feature").Value != nil {
			require.Positive(t, connections.Load(),
				"the stream has to be connected, or this passes for the wrong reason")
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Fail(t, "the initial load was never retried, so the client stayed empty")
}

func TestSseConnectsAfterAFailedFirstLoad(t *testing.T) {
	features := `{"feature":{"defaultValue":1}}`

	var apiCalls atomic.Int32
	stream := sseResponse(features, 10*time.Millisecond, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features/somekey":
			if apiCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Add("x-sse-support", "enabled")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"features":%s}`, features)))
		case "/sub/somekey":
			stream(r.Context(), w)
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.URL),
		WithClientKey("somekey"),
		WithSseDataSource(),
	)
	require.NoError(t, err, "NewClient does not surface a data source start failure")
	require.NotNil(t, client)
	defer func() { _ = client.Close() }()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if client.EvalFeature(ctx, "feature").Value != nil {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Fail(t, "the SSE stream never connected after the first load failed")
}
