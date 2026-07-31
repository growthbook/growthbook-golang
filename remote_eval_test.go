package growthbook

import (
	"context"
	"net/http"
	"testing"

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
