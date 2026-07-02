package growthbook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInMemoryFeatureCacheGetSet(t *testing.T) {
	cache := NewInMemoryFeatureCache()

	_, ok := cache.Get(ctx, "missing")
	require.False(t, ok)

	entry := &FeatureCacheEntry{
		Payload: json.RawMessage(`{"features":{"feat":{"defaultValue":"v"}}}`),
		Etag:    "e1",
	}
	cache.Set(ctx, "k", entry)

	got, ok := cache.Get(ctx, "k")
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

	entry, ok := cache.Get(ctx, "https://example.com::k")
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

	entry, ok := cache.Get(ctx, "https://example.com::k")
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

func (c *jsonRoundTripCache) Get(_ context.Context, key string) (*FeatureCacheEntry, bool) {
	b, ok := c.store[key]
	if !ok {
		return nil, false
	}
	var entry FeatureCacheEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil, false
	}
	return &entry, true
}

func (c *jsonRoundTripCache) Set(_ context.Context, key string, entry *FeatureCacheEntry) {
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	c.store[key] = b
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
