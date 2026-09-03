package growthbook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInMemoryFeatureCacheGetSet(t *testing.T) {
	cache := NewInMemoryFeatureCache()

	_, ok, err := cache.Get(ctx, "missing")
	require.NoError(t, err)
	require.False(t, ok)

	entry := &FeatureCacheEntry{
		Payload: json.RawMessage(`{"features":{"feat":{"defaultValue":"v"}}}`),
		Etag:    "e1",
	}
	require.NoError(t, cache.Set(ctx, "k", entry))

	got, ok, err := cache.Get(ctx, "k")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "e1", got.Etag)
}

func TestSeedsFeaturesFromCacheOnStartup(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	cache.Set(ctx, "https://example.com::k", &FeatureCacheEntry{
		Payload: json.RawMessage(`{"features":{"feat":{"defaultValue":"cached"}}}`),
		Etag:    "e1",
	})

	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache))
	require.NoError(t, err)

	res := client.EvalFeature(ctx, "feat")
	require.Equal(t, "cached", res.Value)
	require.Equal(t, DefaultValueResultSource, res.Source)
}

func TestWriteThroughToCacheOnUpdate(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache))
	require.NoError(t, err)

	payload := `{"features":{"feat":{"defaultValue":"fromApi"}},"dateUpdated":"2020-01-01T00:00:00Z"}`
	resp := &FeatureApiResponse{Etag: "etag123", raw: json.RawMessage(payload)}
	require.NoError(t, json.Unmarshal([]byte(payload), resp))
	require.NoError(t, client.updateFromApiResponse(ctx, resp))

	entry, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "etag123", entry.Etag)
	require.JSONEq(t, payload, string(entry.Payload))
}

func TestWriteThroughPreservesEtagWhenUpdateHasNone(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache))
	require.NoError(t, err)

	// First update carries an etag (e.g. from a poll response).
	p1 := `{"features":{"feat":{"defaultValue":1}},"dateUpdated":"2020-01-01T00:00:00Z"}`
	r1 := &FeatureApiResponse{Etag: "etag-1", raw: json.RawMessage(p1)}
	require.NoError(t, json.Unmarshal([]byte(p1), r1))
	require.NoError(t, client.updateFromApiResponse(ctx, r1))

	// Second update has no etag (e.g. an SSE event) — must not clobber it.
	p2 := `{"features":{"feat":{"defaultValue":2}},"dateUpdated":"2020-01-02T00:00:00Z"}`
	require.NoError(t, client.UpdateFromApiResponseJSON(p2))

	entry, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "etag-1", entry.Etag)
}

func TestInlineFeaturesNotOverwrittenByCache(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	cache.Set(ctx, "https://example.com::k", &FeatureCacheEntry{
		Payload: json.RawMessage(`{"features":{"feat":{"defaultValue":"cached"}}}`),
	})

	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache),
		WithJsonFeatures(`{"feat":{"defaultValue":"inline"}}`))
	require.NoError(t, err)

	res := client.EvalFeature(ctx, "feat")
	require.Equal(t, "inline", res.Value)
}

func TestNoCacheConfiguredIsNoop(t *testing.T) {
	// Without WithFeatureCache, updates must not panic and evaluation works.
	client, err := NewClient(ctx, WithClientKey("k"))
	require.NoError(t, err)
	require.NoError(t, client.UpdateFromApiResponseJSON(
		`{"features":{"feat":{"defaultValue":true}},"dateUpdated":"2020-01-01T00:00:00Z"}`))
	require.True(t, client.EvalFeature(ctx, "feat").On)
}

// jsonRoundTripCache simulates an external backend (e.g. Redis) that serializes
// entries to JSON and back, to prove FeatureCacheEntry survives a round-trip.
type jsonRoundTripCache struct{ store map[string][]byte }

func (c *jsonRoundTripCache) Get(_ context.Context, key string) (*FeatureCacheEntry, bool, error) {
	b, ok := c.store[key]
	if !ok {
		return nil, false, nil
	}
	var entry FeatureCacheEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil, false, err
	}
	return &entry, true, nil
}

func (c *jsonRoundTripCache) Set(_ context.Context, key string, entry *FeatureCacheEntry) error {
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	c.store[key] = b
	return nil
}

func TestCachedFeatureWithConditionSurvivesJSONRoundTrip(t *testing.T) {
	cache := &jsonRoundTripCache{store: map[string][]byte{}}
	cache.Set(ctx, "https://example.com::k", &FeatureCacheEntry{
		Payload: json.RawMessage(
			`{"features":{"feat":{"defaultValue":"off","rules":[{"condition":{"country":"US"},"force":"on"}]}}}`),
	})

	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithAttributes(Attributes{"country": "US"}),
		WithFeatureCache(cache))
	require.NoError(t, err)

	// If the condition survived serialization, the rule forces "on" for US.
	res := client.EvalFeature(ctx, "feat")
	require.Equal(t, "on", res.Value)
	require.Equal(t, ForceResultSource, res.Source)
}

// recordingCache counts Set calls so tests can assert seeding does not write back.
type recordingCache struct {
	entry *FeatureCacheEntry
	sets  int
}

func (c *recordingCache) Get(context.Context, string) (*FeatureCacheEntry, bool, error) {
	if c.entry == nil {
		return nil, false, nil
	}
	return c.entry, true, nil
}

func (c *recordingCache) Set(_ context.Context, _ string, e *FeatureCacheEntry) error {
	c.sets++
	c.entry = e
	return nil
}

func TestSeedDoesNotWriteBackToCache(t *testing.T) {
	cache := &recordingCache{entry: &FeatureCacheEntry{
		Payload: json.RawMessage(`{"features":{"feat":{"defaultValue":"cached"}},"dateUpdated":"2020-01-01T00:00:00Z"}`),
		Etag:    "e1",
	}}
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"), WithClientKey("k"), WithFeatureCache(cache))
	require.NoError(t, err)

	// The cache was adopted...
	require.Equal(t, "cached", client.EvalFeature(ctx, "feat").Value)
	// ...but seeding must not write back, which would refresh a TTL backend's expiry.
	require.Equal(t, 0, cache.sets)
}

// errorCache always fails, simulating a backend outage.
type errorCache struct{}

func (errorCache) Get(context.Context, string) (*FeatureCacheEntry, bool, error) {
	return nil, false, errors.New("backend down")
}
func (errorCache) Set(context.Context, string, *FeatureCacheEntry) error {
	return errors.New("backend down")
}

func TestCacheBackendErrorsAreNonFatal(t *testing.T) {
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"), WithClientKey("k"),
		WithFeatureCache(errorCache{}), withSilentTestLogger())
	require.NoError(t, err) // a Get failure on startup must not fail construction

	// A Set failure during an update is logged, not propagated, and the update applies.
	require.NoError(t, client.UpdateFromApiResponseJSON(
		`{"features":{"feat":{"defaultValue":"api"}},"dateUpdated":"2020-01-02T00:00:00Z"}`))
	require.Equal(t, "api", client.EvalFeature(ctx, "feat").Value)
}

func TestPollIgnoresCachedEtagWhenInlineFeaturesUsed(t *testing.T) {
	var sentEtag string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sentEtag = r.Header.Get("If-None-Match")
		if sentEtag == "cached-etag" {
			// The bug: a 304 here would freeze the unrelated inline features.
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("etag", "fresh-etag")
		_, _ = w.Write([]byte(`{"features":{"feat":{"defaultValue":"fromApi"}},"dateUpdated":"2100-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	cache := NewInMemoryFeatureCache()
	require.NoError(t, cache.Set(ctx, srv.URL+"::k", &FeatureCacheEntry{
		Payload: json.RawMessage(`{"features":{"feat":{"defaultValue":"cached"}}}`),
		Etag:    "cached-etag",
	}))

	client, err := NewClient(ctx,
		withSilentTestLogger(),
		WithHttpClient(srv.Client()), WithApiHost(srv.URL), WithClientKey("k"),
		WithFeatureCache(cache),
		WithJsonFeatures(`{"feat":{"defaultValue":"inline"}}`), // inline → seed is skipped
		WithPollDataSource(time.Hour))
	require.NoError(t, err)
	require.NoError(t, client.EnsureLoaded(ctx))

	// Seed was skipped, so poll must not reuse the cached etag; it fetches fresh.
	require.Empty(t, sentEtag)
	require.Equal(t, "fromApi", client.EvalFeature(ctx, "feat").Value)
}

func TestWriteThroughStampsUpdatedAt(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"), WithClientKey("k"), WithFeatureCache(cache))
	require.NoError(t, err)

	before := time.Now().UTC()
	require.NoError(t, client.UpdateFromApiResponseJSON(
		`{"features":{"feat":{"defaultValue":"v"}},"dateUpdated":"2020-01-01T00:00:00Z"}`))

	entry, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.True(t, ok)
	// A backend with no expiry of its own (e.g. a file) ages entries out by this.
	require.False(t, entry.UpdatedAt.Before(before))
	require.Equal(t, time.UTC, entry.UpdatedAt.Location())
}

func TestSeedDoesNotDependOnUpdatedAt(t *testing.T) {
	// Entries written before UpdatedAt existed (zero value) must still seed.
	cache := NewInMemoryFeatureCache()
	require.NoError(t, cache.Set(ctx, "https://example.com::k", &FeatureCacheEntry{
		Payload: json.RawMessage(`{"features":{"feat":{"defaultValue":"cached"}}}`),
	}))

	client, err := NewClient(ctx,
		WithApiHost("https://example.com"), WithClientKey("k"), WithFeatureCache(cache))
	require.NoError(t, err)
	require.Equal(t, "cached", client.EvalFeature(ctx, "feat").Value)
}
