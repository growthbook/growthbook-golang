package growthbook

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stalledServer accepts requests and never answers until the test ends.
func stalledServer(t *testing.T) *httptest.Server {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv
}

// stalledBodyServer sends the response headers and part of the body, flushes
// them, then never sends the rest until the test ends. A request to it gets
// past the headers and stalls in the body read.
func stalledBodyServer(t *testing.T) *httptest.Server {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"features": {"foo": {"defaultValue": "fro`))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv
}

func TestRequestTimeout(t *testing.T) {
	ctx := context.TODO()

	t.Run("defaults to a finite timeout", func(t *testing.T) {
		client, err := NewClient(ctx)
		require.NoError(t, err)
		require.Equal(t, defaultRequestTimeout, client.data.requestTimeout)
	})

	t.Run("rejects a negative timeout", func(t *testing.T) {
		_, err := NewClient(ctx, WithRequestTimeout(-time.Second))
		require.Error(t, err)
	})

	t.Run("refresh fails fast and keeps inline features", func(t *testing.T) {
		srv := stalledServer(t)
		logger, _ := testLogger(slog.LevelError, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithApiHost(srv.URL),
			WithClientKey("somekey"),
			WithJsonFeatures(`{"foo": {"defaultValue": "inline"}}`),
			WithRequestTimeout(50*time.Millisecond),
		)
		require.NoError(t, err)

		start := time.Now()
		err = client.RefreshFeatures(context.Background())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(start), 2*time.Second)
		require.Equal(t, "inline", client.EvalFeature(ctx, "foo").Value)
	})

	// The headers arrive in time, so this times out in io.ReadAll rather than
	// in Do: the timeout has to cover the whole body, not just the headers.
	t.Run("bounds a body that stalls after the headers", func(t *testing.T) {
		srv := stalledBodyServer(t)
		logger, _ := testLogger(slog.LevelError, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithApiHost(srv.URL),
			WithClientKey("somekey"),
			WithJsonFeatures(`{"foo": {"defaultValue": "inline"}}`),
			WithRequestTimeout(50*time.Millisecond),
		)
		require.NoError(t, err)

		done := make(chan error, 1)
		go func() { done <- client.RefreshFeatures(context.Background()) }()
		select {
		case err = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("RefreshFeatures still waiting on a response body that stalled after the headers")
		}
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, "inline", client.EvalFeature(ctx, "foo").Value)
	})

	t.Run("applies with a custom http client", func(t *testing.T) {
		srv := stalledServer(t)
		logger, _ := testLogger(slog.LevelError, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithHttpClient(&http.Client{}),
			WithApiHost(srv.URL),
			WithClientKey("somekey"),
			WithRequestTimeout(50*time.Millisecond),
		)
		require.NoError(t, err)

		err = client.RefreshFeatures(context.Background())
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("bounds the first load of a data source", func(t *testing.T) {
		srv := stalledServer(t)
		logger, _ := testLogger(slog.LevelError, t)
		client, err := NewClient(ctx,
			WithLogger(logger),
			WithApiHost(srv.URL),
			WithClientKey("somekey"),
			WithPollDataSource(time.Minute),
			WithRequestTimeout(50*time.Millisecond),
		)
		require.NoError(t, err)
		defer client.Close()

		start := time.Now()
		err = client.EnsureLoaded(context.Background())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("zero disables the timeout", func(t *testing.T) {
		for _, tc := range []struct {
			name         string
			opts         []ClientOption
			wantDeadline bool
		}{
			{"default", nil, true},
			{"zero", []ClientOption{WithRequestTimeout(0)}, false},
		} {
			var hasDeadline bool
			transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				_, hasDeadline = req.Context().Deadline()
				return nil, errors.New("stop")
			})
			opts := append([]ClientOption{
				WithHttpClient(&http.Client{Transport: transport}),
				WithClientKey("somekey"),
			}, tc.opts...)
			client, err := NewClient(ctx, opts...)
			require.NoError(t, err)

			_ = client.RefreshFeatures(context.Background())
			require.Equal(t, tc.wantDeadline, hasDeadline, tc.name)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
