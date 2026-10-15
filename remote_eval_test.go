package growthbook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newRemoteEvalClient(t *testing.T, ts *testServer, opts ...ClientOption) *Client {
	t.Helper()
	base := []ClientOption{
		WithHttpClient(ts.http.Client()),
		WithApiHost(ts.http.URL),
		WithClientKey("k"),
		WithRemoteEval(true),
	}
	client, err := NewClient(context.TODO(), append(base, opts...)...)
	require.NoError(t, err)
	return client
}

func TestRemoteEvalConfigValidation(t *testing.T) {
	ctx := context.TODO()

	_, err := NewClient(ctx, WithRemoteEval(true))
	require.ErrorIs(t, err, ErrRemoteEvalNoClientKey)

	_, err = NewClient(ctx, WithRemoteEval(true), WithClientKey("k"), WithDecryptionKey("key"))
	require.ErrorIs(t, err, ErrRemoteEvalDecryptionKey)

	_, err = NewClient(ctx, WithRemoteEval(true), WithClientKey("k"), WithSseDataSource())
	require.ErrorIs(t, err, ErrRemoteEvalWithDataSource)
}

func TestRemoteEvalEvaluates(t *testing.T) {
	ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	defer ts.http.Close()

	client := newRemoteEvalClient(t, ts, WithAttributes(Attributes{"id": "1"}))
	require.NoError(t, client.EnsureLoaded(ctx))

	res := client.EvalFeature(ctx, "feat")
	require.Equal(t, "on", res.Value)
	require.Equal(t, DefaultValueResultSource, res.Source)
}

func TestRemoteEvalFeaturesReturnsLoadedSet(t *testing.T) {
	ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	defer ts.http.Close()

	client := newRemoteEvalClient(t, ts, WithAttributes(Attributes{"id": "1"}))
	require.Empty(t, client.Features())

	require.NoError(t, client.EnsureLoaded(ctx))

	require.Contains(t, client.Features(), "feat")
}

func TestRemoteEvalCachesByAttributes(t *testing.T) {
	ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	defer ts.http.Close()

	client := newRemoteEvalClient(t, ts, WithAttributes(Attributes{"id": "1"}))

	// Repeated evaluations for the same attributes hit the cache: one request.
	client.EvalFeature(ctx, "feat")
	client.EvalFeature(ctx, "feat")
	require.Equal(t, int32(1), ts.count.Load())

	// A different attribute set triggers a new remote request.
	child, err := client.WithAttributes(Attributes{"id": "2"})
	require.NoError(t, err)
	child.EvalFeature(ctx, "feat")
	require.Equal(t, int32(2), ts.count.Load())
}

func TestRemoteEvalCacheKeyAttributes(t *testing.T) {
	ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	defer ts.http.Close()

	client := newRemoteEvalClient(t, ts,
		WithAttributes(Attributes{"id": "1", "page": "home"}),
		WithCacheKeyAttributes([]string{"id"}),
	)
	client.EvalFeature(ctx, "feat")
	require.Equal(t, int32(1), ts.count.Load())

	// Changing a non-key attribute must NOT trigger a new request.
	sameKey, err := client.WithAttributes(Attributes{"id": "1", "page": "pricing"})
	require.NoError(t, err)
	sameKey.EvalFeature(ctx, "feat")
	require.Equal(t, int32(1), ts.count.Load())

	// Changing the key attribute triggers a new request.
	newKey, err := client.WithAttributes(Attributes{"id": "2", "page": "home"})
	require.NoError(t, err)
	newKey.EvalFeature(ctx, "feat")
	require.Equal(t, int32(2), ts.count.Load())
}

func TestRemoteEvalFailureFallsBackToUnknown(t *testing.T) {
	ts := startServer(http.StatusInternalServerError, []byte(""))
	defer ts.http.Close()

	client := newRemoteEvalClient(t, ts, WithAttributes(Attributes{"id": "1"}))

	// EnsureLoaded surfaces the error.
	require.Error(t, client.EnsureLoaded(ctx))

	// Evaluation does not panic and reports the feature as unknown.
	res := client.EvalFeature(ctx, "feat")
	require.Equal(t, UnknownFeatureResultSource, res.Source)
	require.False(t, res.On)
}

func TestRemoteEvalIgnoresNilFeaturesResponse(t *testing.T) {
	ts := startServer(http.StatusOK, []byte(`{"dateUpdated":"2020-01-01T00:00:00Z"}`))
	defer ts.http.Close()

	client := newRemoteEvalClient(t, ts, WithAttributes(Attributes{"id": "1"}))
	require.NoError(t, client.EnsureLoaded(ctx))

	// A malformed (no "features") response is not cached and does not panic.
	res := client.EvalFeature(ctx, "feat")
	require.Equal(t, UnknownFeatureResultSource, res.Source)
}

func TestRemoteEvalRefetchesAfterTTL(t *testing.T) {
	ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	defer ts.http.Close()

	client := newRemoteEvalClient(t, ts,
		WithAttributes(Attributes{"id": "1"}),
		WithRemoteEvalTTL(time.Minute),
	)
	base := time.Unix(1000, 0)
	cur := base
	client.data.now = func() time.Time { return cur }

	client.EvalFeature(ctx, "feat")
	require.Equal(t, int32(1), ts.count.Load())

	// Within TTL: no new request.
	cur = base.Add(30 * time.Second)
	client.EvalFeature(ctx, "feat")
	require.Equal(t, int32(1), ts.count.Load())

	// Past TTL: a new request is made.
	cur = base.Add(2 * time.Minute)
	client.EvalFeature(ctx, "feat")
	require.Equal(t, int32(2), ts.count.Load())
}

func TestRemoteEvalRefreshFeaturesBypassesCache(t *testing.T) {
	var value atomic.Value
	value.Store("v1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"features":{"feat":{"defaultValue":"` + value.Load().(string) + `"}}}`))
	}))
	defer srv.Close()

	// TTL 0 never expires, so only an explicit refresh can pick up new values.
	client, err := NewClient(ctx,
		WithHttpClient(srv.Client()), WithApiHost(srv.URL), WithClientKey("k"),
		WithRemoteEval(true), WithAttributes(Attributes{"id": "1"}), WithRemoteEvalTTL(0),
	)
	require.NoError(t, err)
	require.NoError(t, client.EnsureLoaded(ctx))
	require.Equal(t, "v1", client.EvalFeature(ctx, "feat").Value)

	// The server changes; the cached result is still served.
	value.Store("v2")
	require.Equal(t, "v1", client.EvalFeature(ctx, "feat").Value)

	// RefreshFeatures re-fetches the remote evaluation despite the fresh cache.
	require.NoError(t, client.RefreshFeatures(ctx))
	require.Equal(t, "v2", client.EvalFeature(ctx, "feat").Value)
}

func TestRemoteEvalServesStaleOnFailure(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	}))
	defer srv.Close()

	client, err := NewClient(ctx,
		WithHttpClient(srv.Client()), WithApiHost(srv.URL), WithClientKey("k"),
		WithRemoteEval(true), WithAttributes(Attributes{"id": "1"}), WithRemoteEvalTTL(time.Minute),
	)
	require.NoError(t, err)
	base := time.Unix(1000, 0)
	cur := base
	client.data.now = func() time.Time { return cur }

	require.Equal(t, "on", client.EvalFeature(ctx, "feat").Value)

	// Endpoint fails and the entry has expired: the stale value is still served.
	fail.Store(true)
	cur = base.Add(2 * time.Minute)
	require.Equal(t, "on", client.EvalFeature(ctx, "feat").Value)
}

func TestRemoteEvalCacheKeyIncludesAllInputs(t *testing.T) {
	// Same attributes but different forced variations / URL must not reuse a
	// cached result computed for different inputs.
	t.Run("forced variations", func(t *testing.T) {
		ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
		defer ts.http.Close()

		client := newRemoteEvalClient(t, ts, WithAttributes(Attributes{"id": "1"}))
		client.EvalFeature(ctx, "feat")
		require.Equal(t, int32(1), ts.count.Load())

		forced, err := client.WithForcedVariations(ForcedVariationsMap{"exp": 1})
		require.NoError(t, err)
		forced.EvalFeature(ctx, "feat")
		require.Equal(t, int32(2), ts.count.Load())
	})

	t.Run("url", func(t *testing.T) {
		ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
		defer ts.http.Close()

		client := newRemoteEvalClient(t, ts, WithAttributes(Attributes{"id": "1"}))
		client.EvalFeature(ctx, "feat")
		require.Equal(t, int32(1), ts.count.Load())

		withURL, err := client.WithUrl("https://example.com/pricing")
		require.NoError(t, err)
		withURL.EvalFeature(ctx, "feat")
		require.Equal(t, int32(2), ts.count.Load())
	})
}

func TestRemoteEvalCacheEviction(t *testing.T) {
	ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	defer ts.http.Close()

	root := newRemoteEvalClient(t, ts, WithRemoteEvalCacheSize(2))

	eval := func(id string) {
		c, err := root.WithAttributes(Attributes{"id": id})
		require.NoError(t, err)
		c.EvalFeature(ctx, "feat")
	}

	eval("1") // cache: [1]
	eval("2") // cache: [2,1]
	eval("3") // cache: [3,2], evicts 1
	require.Equal(t, int32(3), ts.count.Load())

	// 2 and 3 are still cached (no new request).
	eval("2")
	eval("3")
	require.Equal(t, int32(3), ts.count.Load())

	// 1 was evicted, so it triggers a fresh request.
	eval("1")
	require.Equal(t, int32(4), ts.count.Load())
}

func TestRemoteEvalChildValidation(t *testing.T) {
	// A child that enables remote eval must enforce the same rules as NewClient.
	withKey, err := NewClient(ctx, WithClientKey("k"), WithDecryptionKey("dk"))
	require.NoError(t, err)
	_, err = withKey.WithRemoteEval(true)
	require.ErrorIs(t, err, ErrRemoteEvalDecryptionKey)

	noKey, err := NewClient(ctx)
	require.NoError(t, err)
	_, err = noKey.WithRemoteEval(true)
	require.ErrorIs(t, err, ErrRemoteEvalNoClientKey)
}

func TestRemoteEvalPayloadForcedVariationsNotNull(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"features":{}}`))
	}))
	defer srv.Close()

	client, err := NewClient(ctx,
		WithHttpClient(srv.Client()), WithApiHost(srv.URL), WithClientKey("k"),
		WithRemoteEval(true), WithAttributes(Attributes{"id": "1"}),
	)
	require.NoError(t, err)
	require.NoError(t, client.EnsureLoaded(ctx))

	require.Contains(t, string(body), `"forcedVariations":{}`)
	require.NotContains(t, string(body), `"forcedVariations":null`)
}

func TestRemoteEvalConcurrentRefreshNoRace(t *testing.T) {
	ts := startServer(http.StatusOK, []byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	defer ts.http.Close()

	// A tiny TTL forces frequent refreshes; run under -race to catch mutation of
	// a shared cache entry concurrently with readers.
	client := newRemoteEvalClient(t, ts,
		WithAttributes(Attributes{"id": "1"}),
		WithRemoteEvalTTL(time.Millisecond),
	)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				time.Sleep(time.Millisecond)
				require.Equal(t, "on", client.EvalFeature(ctx, "feat").Value)
			}
		}()
	}
	wg.Wait()
}

func TestRemoteEvalSingleFlight(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		time.Sleep(20 * time.Millisecond) // hold the request so others overlap
		_, _ = w.Write([]byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	}))
	defer srv.Close()

	client, err := NewClient(ctx,
		WithHttpClient(srv.Client()), WithApiHost(srv.URL), WithClientKey("k"),
		WithRemoteEval(true), WithAttributes(Attributes{"id": "1"}),
	)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.EvalFeature(ctx, "feat")
		}()
	}
	wg.Wait()

	// Concurrent evaluations for the same key coalesce into one remote request.
	require.Equal(t, int32(1), count.Load())
}

func TestRemoteEvalSingleFlightSharesFailure(t *testing.T) {
	// The first request blocks until released, so the other callers are
	// guaranteed to be waiting on it when it fails.
	var count atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newRemoteEvalClient(t, &testServer{http: srv}, WithAttributes(Attributes{"id": "1"}))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = client.EnsureLoaded(ctx)
	}()
	<-started
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.EnsureLoaded(ctx)
		}()
	}
	time.Sleep(50 * time.Millisecond) // let the waiters queue up
	close(release)
	wg.Wait()

	// Callers that waited on the failed request share its error instead of
	// each retrying in turn.
	require.Equal(t, int32(1), count.Load())
}

func TestRemoteEvalWaiterHonorsContext(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		<-release
		_, _ = w.Write([]byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	}))
	defer srv.Close()
	defer close(release) // runs before srv.Close, unblocking the hung request

	client := newRemoteEvalClient(t, &testServer{http: srv}, WithAttributes(Attributes{"id": "1"}))

	// The first caller hangs on a slow request.
	go func() { _ = client.EnsureLoaded(context.Background()) }()
	<-started

	// A second caller for the same key must give up when its own context ends.
	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.EnsureLoaded(waitCtx) }()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("waiter ignored its context and stayed blocked")
	}
}

func TestRemoteEvalUsesFetchedResultDespiteEviction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	}))
	defer srv.Close()

	// A one-entry cache shared by many children with distinct attributes: each
	// child's entry is likely evicted by another child right after it loads.
	client := newRemoteEvalClient(t, &testServer{http: srv}, WithRemoteEvalCacheSize(1))

	var unknown atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		child, err := client.WithAttributes(Attributes{"id": i})
		require.NoError(t, err)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if child.EvalFeature(ctx, "feat").Source == UnknownFeatureResultSource {
					unknown.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	// Every evaluation uses the result it just fetched, even if the shared
	// cache evicted it before the evaluation read it back.
	require.Zero(t, unknown.Load())
}

func TestRemoteEvalFetchTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // the server accepts the request and never answers
	}))
	defer srv.Close()
	defer close(release) // runs before srv.Close, unblocking the hung handler

	client := newRemoteEvalClient(t, &testServer{http: srv},
		WithAttributes(Attributes{"id": "1"}),
		WithRemoteEvalTimeout(50*time.Millisecond),
	)

	// A caller without a deadline must still get an error once the fetch
	// timeout elapses, instead of hanging on the silent server.
	done := make(chan error, 1)
	go func() { done <- client.EnsureLoaded(context.Background()) }()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("remote-eval fetch ignored its timeout and stayed blocked")
	}
}

func TestRemoteEvalLeaderCancelDoesNotFailWaiters(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		<-release
		_, _ = w.Write([]byte(`{"features":{"feat":{"defaultValue":"on"}}}`))
	}))
	defer srv.Close()

	client := newRemoteEvalClient(t, &testServer{http: srv}, WithAttributes(Attributes{"id": "1"}))

	// The leader starts the fetch, then a second caller joins it.
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() { leaderDone <- client.EnsureLoaded(leaderCtx) }()
	<-started
	waiterDone := make(chan error, 1)
	go func() { waiterDone <- client.EnsureLoaded(context.Background()) }()
	time.Sleep(50 * time.Millisecond) // let the waiter join the in-flight fetch

	// The leader's caller gives up: the leader returns promptly...
	cancelLeader()
	select {
	case err := <-leaderDone:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("leader ignored its own context")
	}

	// ...but the shared fetch keeps running for the waiter, which succeeds.
	close(release)
	select {
	case err := <-waiterDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("waiter never received the shared result")
	}
	require.Equal(t, "on", client.EvalFeature(ctx, "feat").Value)
}
