package growthbook

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryDelayFollowsTheSpecifiedSequence(t *testing.T) {
	const interval = 30 * time.Second

	require.Equal(t, interval, retryDelay(interval, 0), "a healthy poller waits its configured interval")

	require.Equal(t, 1*time.Second, retryDelay(interval, 1))
	require.Equal(t, 2*time.Second, retryDelay(interval, 2))
	require.Equal(t, 4*time.Second, retryDelay(interval, 3))
	require.Equal(t, 8*time.Second, retryDelay(interval, 4))
	require.Equal(t, 16*time.Second, retryDelay(interval, 5))
}

func TestRetriesAreSpentAfterTheBoundAndPollingResumes(t *testing.T) {
	const interval = 30 * time.Second

	require.Equal(t, interval, retryDelay(interval, maxRetryAttempts+1),
		"once the quick retries are spent the poller waits out its normal interval")
	require.Equal(t, interval, retryDelay(interval, 100),
		"a sustained outage settles into ordinary polling rather than growing without bound")
}

func TestRetryDelayNeverExceedsThePollInterval(t *testing.T) {
	const interval = 2 * time.Second

	for failures := 1; failures <= maxRetryAttempts; failures++ {
		require.LessOrEqual(t, retryDelay(interval, failures), interval, "failure %d", failures)
	}
}

func TestRetryDelayIsAlwaysPositiveAndBounded(t *testing.T) {
	for _, failures := range []int{-1, 0, 1, 5, 50, 1000} {
		delay := retryDelay(30*time.Second, failures)

		require.Positive(t, delay, "failures %d", failures)
		require.LessOrEqual(t, delay, 30*time.Second, "failures %d", failures)
	}
}

func TestPollingRecoversAfterAFailedFetch(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := func() (int, []byte) {
			attempts++

			if attempts == 1 {
				return http.StatusInternalServerError, nil
			}

			return http.StatusOK, []byte(`{
      "features": {"feature": {"defaultValue": 1}},
      "dateUpdated": "2026-01-01T00:00:00Z"
    }`)
		}()

		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.URL),
		WithClientKey("test-key"),
		WithPollDataSource(50*time.Millisecond),
	)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		if client.EvalFeature(ctx, "feature").Value != nil {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Fail(t, "the poller never recovered after the first fetch failed")
}

func TestExhaustedRetriesKeepServingCachedFeatures(t *testing.T) {
	const interval = 50 * time.Millisecond

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
        "features": {"feature": {"defaultValue": "cached"}},
        "dateUpdated": "2026-01-01T00:00:00Z"
      }`))
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.URL),
		WithClientKey("test-key"),
		WithPollDataSource(interval),
	)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	require.NoError(t, client.EnsureLoaded(ctx))
	require.Equal(t, "cached", client.EvalFeature(ctx, "feature").Value)

	time.Sleep(20 * interval)

	require.Equal(t, "cached", client.EvalFeature(ctx, "feature").Value,
		"a failing API must not wipe out the best values the client already has")

	made := attempts.Load()

	require.Greater(t, made, int32(2), "the poller keeps trying after the retries are exhausted")
	require.Less(t, made, int32(60), "and it does so on the interval rather than in a tight loop")
}
