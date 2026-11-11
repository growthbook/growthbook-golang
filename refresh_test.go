package growthbook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var refreshFeaturesJSON = []byte(`{
  "features": {"foo": {"defaultValue": "api"}},
  "dateUpdated": "2030-01-01T00:00:00Z"
}`)

// fastRetry keeps retry tests quick while still exercising the backoff.
var fastRetry = RetryPolicy{MaxAttempts: 5, InitialDelay: time.Millisecond, MaxDelay: 4 * time.Millisecond}

// flakyServer answers failCode for its first failures requests, then 200.
type flakyServer struct {
	http     *httptest.Server
	count    atomic.Int32
	lastReq  atomic.Pointer[http.Request]
	failures int32
	failCode int
}

func startFlakyServer(failures int32, failCode int) *flakyServer {
	ts := &flakyServer{failures: failures, failCode: failCode}
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.lastReq.Store(r)
		if ts.count.Add(1) <= ts.failures {
			w.WriteHeader(ts.failCode)
			return
		}
		w.Header().Set("etag", `W/"v1"`)
		w.Header().Set("x-sse-support", "enabled")
		_, _ = w.Write(refreshFeaturesJSON)
	}))
	return ts
}

func newRefreshClient(t *testing.T, ts *flakyServer, opts ...ClientOption) *Client {
	t.Helper()
	logger, _ := testLogger(slog.LevelError, t)
	client, err := NewClient(context.Background(), append([]ClientOption{
		WithLogger(logger),
		WithHttpClient(ts.http.Client()),
		WithApiHost(ts.http.URL),
		WithClientKey("somekey"),
	}, opts...)...)
	require.NoError(t, err)
	return client
}

func TestRetryPolicyDelays(t *testing.T) {
	p := DefaultRetryPolicy()
	want := []time.Duration{0, 0, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 16 * time.Second}
	for attempt, delay := range want {
		require.Equal(t, delay, p.delayBeforeAttempt(attempt), "attempt %d", attempt)
	}

	require.Equal(t, time.Duration(0), RetryPolicy{MaxAttempts: 3}.delayBeforeAttempt(3))
	huge := RetryPolicy{MaxAttempts: math.MaxInt, InitialDelay: time.Hour, MaxDelay: math.MaxInt64}
	require.Equal(t, time.Duration(math.MaxInt64), huge.delayBeforeAttempt(1000), "doubling must not overflow")
}

func TestWithRetryPolicyValidates(t *testing.T) {
	for _, p := range []RetryPolicy{
		{MaxAttempts: 0},
		{MaxAttempts: 1, InitialDelay: -1},
		{MaxAttempts: 1, MaxDelay: -1},
	} {
		_, err := NewClient(context.Background(), WithRetryPolicy(p))
		require.Error(t, err, "%+v", p)
	}
	_, err := NewClient(context.Background(), WithRetryPolicy(RetryPolicy{MaxAttempts: 1}))
	require.NoError(t, err)
}

func TestIsRetryableFetchError(t *testing.T) {
	cases := map[error]bool{
		&FeatureApiError{StatusCode: 408}:                                                true,
		&FeatureApiError{StatusCode: 429}:                                                true,
		&FeatureApiError{StatusCode: 500}:                                                true,
		&FeatureApiError{StatusCode: 503}:                                                true,
		&FeatureApiError{StatusCode: 400}:                                                false,
		&FeatureApiError{StatusCode: 401}:                                                false,
		&FeatureApiError{StatusCode: 404}:                                                false,
		&url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: errors.New("refused")}}: true,
		&url.Error{Op: "Get", Err: io.EOF}:                                               true,
		&url.Error{Op: "Get", Err: errors.New(`unsupported protocol scheme "htp"`)}:      false,
		io.ErrUnexpectedEOF:                                                              true,
		fmt.Errorf("invalid character 'x'"):                                              false,
	}
	for err, want := range cases {
		require.Equal(t, want, isRetryableFetchError(err), "%v", err)
	}
}

func TestRefreshFeaturesRetry(t *testing.T) {
	ctx := context.Background()
	features := FeatureMap{"foo": &Feature{DefaultValue: "api"}}

	t.Run("Retries retryable statuses until a fetch succeeds", func(t *testing.T) {
		ts := startFlakyServer(3, http.StatusServiceUnavailable)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithRetryPolicy(fastRetry))

		require.NoError(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(4), ts.count.Load())
		require.Equal(t, features, client.Features())
	})

	t.Run("Gives up after MaxAttempts and returns the last error", func(t *testing.T) {
		ts := startFlakyServer(100, http.StatusTooManyRequests)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithRetryPolicy(fastRetry))

		err := client.RefreshFeatures(ctx)
		var apiErr *FeatureApiError
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
		require.Equal(t, int32(5), ts.count.Load())
		require.Empty(t, client.Features())
	})

	t.Run("Does not retry client errors", func(t *testing.T) {
		ts := startFlakyServer(100, http.StatusUnauthorized)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithRetryPolicy(fastRetry))

		require.Error(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(1), ts.count.Load())
	})

	t.Run("A single attempt policy disables retries", func(t *testing.T) {
		ts := startFlakyServer(100, http.StatusInternalServerError)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithRetryPolicy(RetryPolicy{MaxAttempts: 1}))

		require.Error(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(1), ts.count.Load())
	})

	t.Run("Cancelling the context stops the backoff", func(t *testing.T) {
		ts := startFlakyServer(100, http.StatusInternalServerError)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithRetryPolicy(RetryPolicy{MaxAttempts: 5, InitialDelay: time.Hour, MaxDelay: time.Hour}))

		ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := client.RefreshFeatures(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(start), time.Second)
		require.Equal(t, int32(1), ts.count.Load())
	})
}

func TestRefreshFeaturesMode(t *testing.T) {
	ctx := context.Background()

	t.Run("Default refresh is skipped while features are fresh, force refresh is not", func(t *testing.T) {
		ts := startFlakyServer(0, 0)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithBackgroundFetchInterval(time.Hour))

		require.NoError(t, client.RefreshFeatures(ctx))
		require.NoError(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(1), ts.count.Load())

		require.NoError(t, client.RefreshFeaturesWithMode(ctx, RefreshForce))
		require.Equal(t, int32(2), ts.count.Load())
		req := ts.lastReq.Load()
		require.Equal(t, "no-cache", req.Header.Get("Cache-Control"))
		require.Empty(t, req.Header.Get("If-None-Match"))
	})

	t.Run("Default refresh fetches once the interval has passed", func(t *testing.T) {
		ts := startFlakyServer(0, 0)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithBackgroundFetchInterval(time.Minute))

		require.NoError(t, client.RefreshFeatures(ctx))
		client.data.markFetched(time.Now().Add(-time.Minute))
		require.NoError(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(2), ts.count.Load())
	})

	t.Run("Without an interval every default refresh fetches", func(t *testing.T) {
		ts := startFlakyServer(0, 0)
		defer ts.http.Close()
		client := newRefreshClient(t, ts)

		require.NoError(t, client.RefreshFeatures(ctx))
		require.NoError(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(2), ts.count.Load())
		require.Empty(t, ts.lastReq.Load().Header.Get("Cache-Control"))
	})

	t.Run("A failed fetch does not count as fresh", func(t *testing.T) {
		ts := startFlakyServer(1, http.StatusBadRequest)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithBackgroundFetchInterval(time.Hour))

		require.Error(t, client.RefreshFeatures(ctx))
		require.NoError(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(2), ts.count.Load())
	})
}

func TestDataSourcesRetryFirstLoad(t *testing.T) {
	ctx := context.Background()
	features := FeatureMap{"foo": &Feature{DefaultValue: "api"}}

	t.Run("Polling", func(t *testing.T) {
		ts := startFlakyServer(2, http.StatusBadGateway)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithRetryPolicy(fastRetry), WithPollDataSource(time.Hour))
		defer client.Close()

		require.NoError(t, client.EnsureLoaded(ctx))
		require.Equal(t, int32(3), ts.count.Load())
		require.Equal(t, features, client.Features())
	})

	t.Run("SSE", func(t *testing.T) {
		ts := startFlakyServer(2, http.StatusBadGateway)
		defer ts.http.Close()
		client := newRefreshClient(t, ts, WithRetryPolicy(fastRetry), WithSseDataSource())
		defer client.Close()

		require.NoError(t, client.EnsureLoaded(ctx))
		require.GreaterOrEqual(t, ts.count.Load(), int32(3))
		require.Equal(t, features, client.Features())
	})
}

func TestPollingHonorsBackgroundFetchInterval(t *testing.T) {
	ts := startFlakyServer(0, 0)
	defer ts.http.Close()
	client := newRefreshClient(t, ts,
		WithPollDataSource(5*time.Millisecond),
		WithBackgroundFetchInterval(time.Hour),
	)
	defer client.Close()

	require.NoError(t, client.EnsureLoaded(context.Background()))
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int32(1), ts.count.Load(), "ticks within the interval must not fetch")
}

// gatedServer holds every request until release is closed, then answers 200.
type gatedServer struct {
	http    *httptest.Server
	count   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func startGatedServer() *gatedServer {
	ts := &gatedServer{entered: make(chan struct{}, 100), release: make(chan struct{})}
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.count.Add(1)
		ts.entered <- struct{}{}
		select {
		case <-ts.release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(refreshFeaturesJSON)
	}))
	return ts
}

func (ts *gatedServer) client(t *testing.T) *Client {
	t.Helper()
	return newRefreshClient(t, &flakyServer{http: ts.http})
}

// waitForWaiters blocks until n callers wait on the running refresh of mode.
func waitForWaiters(t *testing.T, client *Client, mode RefreshMode, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		g := &client.data.refreshes
		g.mu.Lock()
		defer g.mu.Unlock()
		call := g.calls[mode]
		return call != nil && call.waiters >= n
	}, time.Second, time.Millisecond)
}

func TestConcurrentRefreshesShareOneFetch(t *testing.T) {
	ctx := context.Background()

	t.Run("Callers of the same mode wait for the running fetch", func(t *testing.T) {
		ts := startGatedServer()
		defer ts.http.Close()
		client := ts.client(t)

		const callers = 10
		errs := make(chan error, callers)
		go func() { errs <- client.RefreshFeatures(ctx) }()
		<-ts.entered
		for i := 1; i < callers; i++ {
			go func() { errs <- client.RefreshFeatures(ctx) }()
		}
		waitForWaiters(t, client, RefreshDefault, callers-1)
		close(ts.release)

		for i := 0; i < callers; i++ {
			require.NoError(t, <-errs)
		}
		require.Equal(t, int32(1), ts.count.Load())
		require.Equal(t, FeatureMap{"foo": &Feature{DefaultValue: "api"}}, client.Features())
	})

	t.Run("A forced refresh does not join a default one", func(t *testing.T) {
		ts := startGatedServer()
		defer ts.http.Close()
		client := ts.client(t)

		errs := make(chan error, 2)
		go func() { errs <- client.RefreshFeatures(ctx) }()
		<-ts.entered
		go func() { errs <- client.RefreshFeaturesWithMode(ctx, RefreshForce) }()
		<-ts.entered
		close(ts.release)

		require.NoError(t, <-errs)
		require.NoError(t, <-errs)
		require.Equal(t, int32(2), ts.count.Load())
	})

	t.Run("A waiter refetches when the leader's context ends", func(t *testing.T) {
		ts := startGatedServer()
		defer ts.http.Close()
		client := ts.client(t)

		leaderCtx, cancelLeader := context.WithCancel(ctx)
		leaderErr := make(chan error, 1)
		go func() { leaderErr <- client.RefreshFeatures(leaderCtx) }()
		<-ts.entered
		waiterErr := make(chan error, 1)
		go func() { waiterErr <- client.RefreshFeatures(ctx) }()
		waitForWaiters(t, client, RefreshDefault, 1)

		cancelLeader()
		require.ErrorIs(t, <-leaderErr, context.Canceled)
		<-ts.entered
		close(ts.release)
		require.NoError(t, <-waiterErr)
		require.Equal(t, int32(2), ts.count.Load())
	})

	t.Run("A waiter shares an HTTP client timeout instead of refetching", func(t *testing.T) {
		ts := startGatedServer()
		defer ts.http.Close()
		defer close(ts.release)
		httpClient := ts.http.Client()
		httpClient.Timeout = 100 * time.Millisecond
		client := newRefreshClient(t, &flakyServer{http: ts.http},
			WithHttpClient(httpClient), WithRetryPolicy(noRetry))

		leaderErr := make(chan error, 1)
		go func() { leaderErr <- client.RefreshFeatures(ctx) }()
		<-ts.entered
		waiterErr := make(chan error, 1)
		go func() { waiterErr <- client.RefreshFeatures(ctx) }()
		waitForWaiters(t, client, RefreshDefault, 1)

		err := <-leaderErr
		require.Error(t, err)
		require.Equal(t, err, <-waiterErr)
		require.Equal(t, int32(1), ts.count.Load())
	})

	t.Run("A waiter's own context stops its wait", func(t *testing.T) {
		ts := startGatedServer()
		defer ts.http.Close()
		defer close(ts.release)
		client := ts.client(t)

		go func() { _ = client.RefreshFeatures(ctx) }()
		<-ts.entered
		waiterCtx, cancel := context.WithCancel(ctx)
		waiterErr := make(chan error, 1)
		go func() { waiterErr <- client.RefreshFeatures(waiterCtx) }()
		waitForWaiters(t, client, RefreshDefault, 1)
		cancel()
		require.ErrorIs(t, <-waiterErr, context.Canceled)
	})
}

func TestCloseStopsDataSourceDuringFirstLoad(t *testing.T) {
	ts := startFlakyServer(1000, http.StatusServiceUnavailable)
	defer ts.http.Close()
	client := newRefreshClient(t, ts,
		WithRetryPolicy(RetryPolicy{MaxAttempts: 5, InitialDelay: time.Hour, MaxDelay: time.Hour}),
		WithPollDataSource(time.Millisecond),
	)
	require.Eventually(t, func() bool { return ts.count.Load() == 1 }, time.Second, time.Millisecond)

	require.NoError(t, client.Close())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.ErrorIs(t, client.EnsureLoaded(ctx), context.Canceled)
	require.Equal(t, int32(1), ts.count.Load())
}

func TestUpdateFromApiResponseKeepsNewestUnderConcurrency(t *testing.T) {
	client, err := NewClient(context.Background())
	require.NoError(t, err)
	base := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	const updates = 50
	var wg sync.WaitGroup
	for i := 0; i < updates; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, client.UpdateFromApiResponse(&FeatureApiResponse{
				Features:    FeatureMap{"foo": &Feature{DefaultValue: float64(i)}},
				DateUpdated: base.Add(time.Duration(i) * time.Minute),
			}))
		}()
	}
	wg.Wait()

	require.Equal(t, base.Add((updates-1)*time.Minute), client.data.getDateUpdated())
	require.Equal(t, float64(updates-1), client.Features()["foo"].DefaultValue)
}

func TestRefreshHandlerWithRetries(t *testing.T) {
	ctx := context.Background()
	dateUpdated := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("A refresh that succeeds after retries reports one update", func(t *testing.T) {
		ts := startFlakyServer(2, http.StatusServiceUnavailable)
		defer ts.http.Close()
		var c refreshCollector
		client := newRefreshClient(t, ts, WithRetryPolicy(fastRetry), WithFeaturesRefreshHandler(c.handler()))

		require.NoError(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(3), ts.count.Load())
		requireEvents(t, c.all(), RefreshResult{Source: RefreshSourceManual, Updated: true, DateUpdated: dateUpdated})
	})

	t.Run("A refresh that runs out of attempts reports one error", func(t *testing.T) {
		ts := startFlakyServer(100, http.StatusTooManyRequests)
		defer ts.http.Close()
		var c refreshCollector
		client := newRefreshClient(t, ts, WithRetryPolicy(fastRetry), WithFeaturesRefreshHandler(c.handler()))

		require.Error(t, client.RefreshFeatures(ctx))
		require.Equal(t, int32(5), ts.count.Load())
		requireEvents(t, c.all(), RefreshResult{Source: RefreshSourceManual, Error: errors.New("any")})
	})

	t.Run("Callers sharing a fetch get one event", func(t *testing.T) {
		ts := startGatedServer()
		defer ts.http.Close()
		var c refreshCollector
		client := newRefreshClient(t, &flakyServer{http: ts.http}, WithFeaturesRefreshHandler(c.handler()))

		const callers = 5
		errs := make(chan error, callers)
		go func() { errs <- client.RefreshFeatures(ctx) }()
		<-ts.entered
		for i := 1; i < callers; i++ {
			go func() { errs <- client.RefreshFeatures(ctx) }()
		}
		waitForWaiters(t, client, RefreshDefault, callers-1)
		close(ts.release)
		for i := 0; i < callers; i++ {
			require.NoError(t, <-errs)
		}
		requireEvents(t, c.all(), RefreshResult{Source: RefreshSourceManual, Updated: true, DateUpdated: dateUpdated})
	})

	t.Run("A refresh skipped as fresh reports nothing", func(t *testing.T) {
		ts := startFlakyServer(0, 0)
		defer ts.http.Close()
		var c refreshCollector
		client := newRefreshClient(t, ts, WithBackgroundFetchInterval(time.Hour), WithFeaturesRefreshHandler(c.handler()))

		require.NoError(t, client.RefreshFeatures(ctx))
		require.NoError(t, client.RefreshFeatures(ctx))
		require.NoError(t, client.RefreshFeaturesWithMode(ctx, RefreshForce))
		require.Equal(t, int32(2), ts.count.Load())
		requireEvents(t, c.all(),
			RefreshResult{Source: RefreshSourceManual, Updated: true, DateUpdated: dateUpdated},
			RefreshResult{Source: RefreshSourceManual, Updated: true, DateUpdated: dateUpdated},
		)
	})
}
