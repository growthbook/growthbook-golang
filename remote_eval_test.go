package growthbook

import (
	"context"
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
