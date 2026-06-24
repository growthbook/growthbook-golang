package growthbook

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func featureMapFromJSON(t *testing.T, s string) FeatureMap {
	t.Helper()
	fm := FeatureMap{}
	require.NoError(t, json.Unmarshal([]byte(s), &fm))
	return fm
}

func TestInMemoryFeatureCacheGetSet(t *testing.T) {
	cache := NewInMemoryFeatureCache()

	_, ok := cache.Get(ctx, "missing")
	require.False(t, ok)

	entry := &FeatureCacheEntry{
		Features:    featureMapFromJSON(t, `{"feat":{"defaultValue":"v"}}`),
		DateUpdated: time.Now(),
		Etag:        "e1",
	}
	cache.Set(ctx, "k", entry)

	got, ok := cache.Get(ctx, "k")
	require.True(t, ok)
	require.Equal(t, "e1", got.Etag)
}

func TestSeedsFeaturesFromCacheOnStartup(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	cache.Set(ctx, "https://example.com::k", &FeatureCacheEntry{
		Features:    featureMapFromJSON(t, `{"feat":{"defaultValue":"cached"}}`),
		DateUpdated: time.Now(),
		Etag:        "e1",
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

	resp := &FeatureApiResponse{
		Features:    featureMapFromJSON(t, `{"feat":{"defaultValue":"fromApi"}}`),
		DateUpdated: time.Now(),
		Etag:        "etag123",
	}
	require.NoError(t, client.UpdateFromApiResponse(resp))

	entry, ok := cache.Get(ctx, "https://example.com::k")
	require.True(t, ok)
	require.Equal(t, "etag123", entry.Etag)
	require.Contains(t, entry.Features, "feat")
}

func TestWriteThroughPreservesEtagWhenUpdateHasNone(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache))
	require.NoError(t, err)

	// First update carries an etag (e.g. from a poll response).
	require.NoError(t, client.UpdateFromApiResponse(&FeatureApiResponse{
		Features:    featureMapFromJSON(t, `{"feat":{"defaultValue":1}}`),
		DateUpdated: time.Now(),
		Etag:        "etag-1",
	}))

	// Second update has no etag (e.g. an SSE event) — must not clobber it.
	require.NoError(t, client.UpdateFromApiResponse(&FeatureApiResponse{
		Features:    featureMapFromJSON(t, `{"feat":{"defaultValue":2}}`),
		DateUpdated: time.Now().Add(time.Second),
	}))

	entry, ok := cache.Get(ctx, "https://example.com::k")
	require.True(t, ok)
	require.Equal(t, "etag-1", entry.Etag)
}

func TestInlineFeaturesNotOverwrittenByCache(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	cache.Set(ctx, "https://example.com::k", &FeatureCacheEntry{
		Features:    featureMapFromJSON(t, `{"feat":{"defaultValue":"cached"}}`),
		DateUpdated: time.Now(),
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
	require.NoError(t, client.UpdateFromApiResponse(&FeatureApiResponse{
		Features:    featureMapFromJSON(t, `{"feat":{"defaultValue":true}}`),
		DateUpdated: time.Now(),
	}))
	require.True(t, client.EvalFeature(ctx, "feat").On)
}
