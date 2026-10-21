package growthbook

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const refreshPayload = `{
  "features": {"feature": {"defaultValue": 1}},
  "dateUpdated": "2026-01-01T00:00:00Z"
}`

type refreshRecorder struct {
	mu       sync.Mutex
	calls    int
	lastResp *FeatureApiResponse
}

func (r *refreshRecorder) subscriber() FeatureRefreshSubscriber {
	return func(_ context.Context, resp *FeatureApiResponse) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls++
		r.lastResp = resp
	}
}

func (r *refreshRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestFeatureRefreshSubscriberIsCalledWhenAPayloadIsApplied(t *testing.T) {
	recorder := &refreshRecorder{}

	client, err := NewClient(ctx)
	require.NoError(t, err)

	unsubscribe := client.SubscribeFeatureRefresh(recorder.subscriber())
	defer unsubscribe()

	require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))

	require.Equal(t, 1, recorder.count())
	require.Contains(t, recorder.lastResp.Features, "feature")
}

func TestSeveralFeatureRefreshSubscribersAreAllCalled(t *testing.T) {
	first := &refreshRecorder{}
	second := &refreshRecorder{}

	client, err := NewClient(ctx)
	require.NoError(t, err)

	defer client.SubscribeFeatureRefresh(first.subscriber())()
	defer client.SubscribeFeatureRefresh(second.subscriber())()

	require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))

	require.Equal(t, 1, first.count())
	require.Equal(t, 1, second.count(), "every subscriber gets each payload, not just the first registered")
}

func TestUnsubscribingStopsOnlyThatSubscriber(t *testing.T) {
	cancelled := &refreshRecorder{}
	remaining := &refreshRecorder{}

	client, err := NewClient(ctx)
	require.NoError(t, err)

	unsubscribe := client.SubscribeFeatureRefresh(cancelled.subscriber())
	defer client.SubscribeFeatureRefresh(remaining.subscriber())()

	require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))
	require.Equal(t, 1, cancelled.count())
	require.Equal(t, 1, remaining.count())

	unsubscribe()

	require.NoError(t, client.UpdateFromApiResponseJSON(`{
    "features": {"feature": {"defaultValue": 2}},
    "dateUpdated": "2026-01-02T00:00:00Z"
  }`))

	require.Equal(t, 1, cancelled.count(), "an unsubscribed function must not be called again")
	require.Equal(t, 2, remaining.count())
}

func TestUnsubscribingTwiceIsHarmless(t *testing.T) {
	client, err := NewClient(ctx)
	require.NoError(t, err)

	unsubscribe := client.SubscribeFeatureRefresh(func(context.Context, *FeatureApiResponse) {})

	unsubscribe()
	require.NotPanics(t, unsubscribe)
}

func TestSubscribingNilIsHarmless(t *testing.T) {
	client, err := NewClient(ctx)
	require.NoError(t, err)

	unsubscribe := client.SubscribeFeatureRefresh(nil)

	require.NotNil(t, unsubscribe)
	require.NotPanics(t, unsubscribe)
	require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))
}

func TestAPanickingSubscriberDoesNotStopTheOthers(t *testing.T) {
	survivor := &refreshRecorder{}

	client, err := NewClient(ctx)
	require.NoError(t, err)

	defer client.SubscribeFeatureRefresh(func(context.Context, *FeatureApiResponse) {
		panic("subscriber blew up")
	})()
	defer client.SubscribeFeatureRefresh(survivor.subscriber())()

	require.NotPanics(t, func() {
		require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))
	}, "a panicking subscriber must not take down the goroutine that delivered the payload")

	require.Equal(t, 1, survivor.count())
}

func TestFeatureRefreshSubscribersAreNotCalledForAStalePayload(t *testing.T) {
	recorder := &refreshRecorder{}

	client, err := NewClient(ctx)
	require.NoError(t, err)

	defer client.SubscribeFeatureRefresh(recorder.subscriber())()

	require.NoError(t, client.UpdateFromApiResponseJSON(`{
    "features": {"feature": {"defaultValue": 1}},
    "dateUpdated": "2026-01-02T00:00:00Z"
  }`))
	require.Equal(t, 1, recorder.count())

	require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))

	require.Equal(t, 1, recorder.count(), "a discarded response changed no features, so there is nothing to report")
}

func TestFeatureRefreshSubscribersAreNotCalledOnMalformedPayload(t *testing.T) {
	recorder := &refreshRecorder{}

	client, err := NewClient(ctx)
	require.NoError(t, err)

	defer client.SubscribeFeatureRefresh(recorder.subscriber())()

	require.Error(t, client.UpdateFromApiResponseJSON(`{ not json`))
	require.Equal(t, 0, recorder.count())
}

func TestFeatureRefreshSubscriberSeesTheNewPayloadAlreadyApplied(t *testing.T) {
	var observed FeatureResult
	var client *Client

	client, err := NewClient(ctx)
	require.NoError(t, err)

	defer client.SubscribeFeatureRefresh(func(cbCtx context.Context, _ *FeatureApiResponse) {
		observed = *client.EvalFeature(cbCtx, "feature")
	})()

	require.NoError(t, client.UpdateFromApiResponseJSON(`{
    "features": {"feature": {"defaultValue": 42}},
    "dateUpdated": "2026-01-01T00:00:00Z"
  }`))

	require.EqualValues(t, 42, observed.Value,
		"the payload must be in effect, and the data lock released, before subscribers are told about it")
}

func TestClientWorksWithNoRefreshSubscribers(t *testing.T) {
	client, err := NewClient(ctx)
	require.NoError(t, err)

	require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))

	require.EqualValues(t, 1, client.EvalFeature(ctx, "feature").Value)
}

func TestFeatureRefreshSubscribersAreCalledFromThePollingDataSource(t *testing.T) {
	recorder := &refreshRecorder{}

	server := startServer(http.StatusOK, []byte(refreshPayload))
	defer server.http.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.http.URL),
		WithClientKey("test-key"),
		WithPollDataSource(50*time.Millisecond),
	)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	defer client.SubscribeFeatureRefresh(recorder.subscriber())()

	require.NoError(t, client.EnsureLoaded(ctx))

	require.GreaterOrEqual(t, recorder.count(), 1,
		"a payload fetched by the data source has to reach subscribers too, not just a direct update")
}

func TestASubscriberMutatingTheResponseCannotChangeEvaluation(t *testing.T) {
	client, err := NewClient(ctx)
	require.NoError(t, err)

	unsubscribe := client.SubscribeFeatureRefresh(func(_ context.Context, resp *FeatureApiResponse) {
		resp.Features["injected"] = &Feature{DefaultValue: "from the subscriber"}
		delete(resp.Features, "feature")
	})
	defer unsubscribe()

	require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))

	require.Equal(t, 1.0, client.EvalFeature(ctx, "feature").Value,
		"a subscriber deleting from the response must not remove a feature from the client")
	require.Nil(t, client.EvalFeature(ctx, "injected").Value,
		"nor must it be able to add one")
}

func TestASubscriberMutatingTheResponseCannotReachAnother(t *testing.T) {
	client, err := NewClient(ctx)
	require.NoError(t, err)

	// Subscribers run in no fixed order, so each one checks what it received before deleting it:
	// with a shared response, whichever runs second finds the feature already gone.
	var sawFeature []bool

	for range 2 {
		defer client.SubscribeFeatureRefresh(func(_ context.Context, resp *FeatureApiResponse) {
			_, ok := resp.Features["feature"]
			delete(resp.Features, "feature")
			sawFeature = append(sawFeature, ok)
		})()
	}

	require.NoError(t, client.UpdateFromApiResponseJSON(refreshPayload))

	require.Equal(t, []bool{true, true}, sawFeature,
		"each subscriber gets its own response - one must not be able to rewrite what another sees")
}

// The client stores a normalized copy of the saved groups, so the response's map never backs live
// state; copying it still keeps one subscriber's writes from reaching the next.
func TestASubscriberMutatingSavedGroupsCannotReachAnother(t *testing.T) {
	const payload = `{
    "features": {"feature": {"defaultValue": 1}},
    "savedGroups": {"admins": ["user-1"]},
    "dateUpdated": "2026-01-01T00:00:00Z"
  }`

	client, err := NewClient(ctx)
	require.NoError(t, err)

	var sawGroup []bool

	for range 2 {
		defer client.SubscribeFeatureRefresh(func(_ context.Context, resp *FeatureApiResponse) {
			_, ok := resp.SavedGroups["admins"]
			delete(resp.SavedGroups, "admins")
			sawGroup = append(sawGroup, ok)
		})()
	}

	require.NoError(t, client.UpdateFromApiResponseJSON(payload))

	require.Equal(t, []bool{true, true}, sawGroup,
		"each subscriber gets its own saved groups - one must not be able to rewrite what another sees")
}

func TestASubscriberMutatingContextualBanditsCannotChangeEvaluation(t *testing.T) {
	const payload = `{
    "features": {"feature": {"defaultValue": 1}},
    "contextualBandits": {"cb-1": {"banditVersion": 1, "contexts": []}},
    "dateUpdated": "2026-01-01T00:00:00Z"
  }`

	client, err := NewClient(ctx)
	require.NoError(t, err)

	unsubscribe := client.SubscribeFeatureRefresh(func(_ context.Context, resp *FeatureApiResponse) {
		delete(resp.ContextualBandits, "cb-1")
	})
	defer unsubscribe()

	require.NoError(t, client.UpdateFromApiResponseJSON(payload))

	client.data.mu.RLock()
	defer client.data.mu.RUnlock()
	require.Contains(t, client.data.contextualBandits, "cb-1",
		"a subscriber's delete must not reach the client's contextual bandit definitions")
}

type refreshCtxKey struct{}

func TestRefreshFeaturesPassesItsContextToSubscribers(t *testing.T) {
	server := startServer(http.StatusOK, []byte(refreshPayload))
	defer server.http.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.http.URL),
		WithClientKey("test-key"),
	)
	require.NoError(t, err)

	var seen any

	defer client.SubscribeFeatureRefresh(func(ctx context.Context, _ *FeatureApiResponse) {
		seen = ctx.Value(refreshCtxKey{})
	})()

	callCtx := context.WithValue(ctx, refreshCtxKey{}, "manual refresh")
	require.NoError(t, client.RefreshFeatures(callCtx))

	require.Equal(t, "manual refresh", seen,
		"a manual refresh has to hand subscribers the caller's context, not a background one")
}
