package growthbook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestInMemoryFeatureCacheCopiesEntries(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	payload := `{"features":{}}`
	entry := &FeatureCacheEntry{Payload: json.RawMessage(payload), Etag: "e1"}
	require.NoError(t, cache.Set(ctx, "k", entry))

	// Changing the entry after Set must not reach the cache.
	entry.Payload[0] = 'X'
	entry.Etag = "changed"

	got, _, err := cache.Get(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, payload, string(got.Payload))
	require.Equal(t, "e1", got.Etag)

	// Neither must changing an entry returned by Get.
	got.Payload[0] = 'X'
	again, _, err := cache.Get(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, payload, string(again.Payload))
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

// A partial response updates only the sections it carries, so the cache merges
// it into what earlier responses supplied instead of replacing them: clients
// seeded from the cache then match the live client.
func TestPartialUpdatesMergeIntoCache(t *testing.T) {
	full := `{"features":{"feat":{"defaultValue":"cached"}},"savedGroups":{"beta":["u1"]},"dateUpdated":"2020-01-01T00:00:00Z"}`

	tests := []struct {
		name    string
		partial string
		want    string
	}{
		{
			"bandit-only keeps features and groups",
			`{"contextualBandits":{},"dateUpdated":"2020-01-02T00:00:00Z"}`,
			`{"features":{"feat":{"defaultValue":"cached"}},"savedGroups":{"beta":["u1"]},"contextualBandits":{},"dateUpdated":"2020-01-02T00:00:00Z"}`,
		},
		{
			"null features keeps features",
			`{"features":null,"dateUpdated":"2020-01-02T00:00:00Z"}`,
			`{"features":{"feat":{"defaultValue":"cached"}},"savedGroups":{"beta":["u1"]},"dateUpdated":"2020-01-02T00:00:00Z"}`,
		},
		{
			"features-only keeps groups",
			`{"features":{"feat":{"defaultValue":"new"}},"dateUpdated":"2020-01-02T00:00:00Z"}`,
			`{"features":{"feat":{"defaultValue":"new"}},"savedGroups":{"beta":["u1"]},"dateUpdated":"2020-01-02T00:00:00Z"}`,
		},
		{
			"groups-only reaches the cache",
			`{"savedGroups":{"beta":["u2"]},"dateUpdated":"2020-01-02T00:00:00Z"}`,
			`{"features":{"feat":{"defaultValue":"cached"}},"savedGroups":{"beta":["u2"]},"dateUpdated":"2020-01-02T00:00:00Z"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewInMemoryFeatureCache()
			client, err := NewClient(ctx,
				WithApiHost("https://example.com"),
				WithClientKey("k"),
				WithFeatureCache(cache))
			require.NoError(t, err)

			require.NoError(t, client.UpdateFromApiResponseJSON(full))
			require.NoError(t, client.UpdateFromApiResponseJSON(tt.partial))

			entry, ok, err := cache.Get(ctx, "https://example.com::k")
			require.NoError(t, err)
			require.True(t, ok)
			require.JSONEq(t, tt.want, string(entry.Payload))

			// A client seeded from the cache sees what the live client sees.
			seeded, err := NewClient(ctx,
				WithApiHost("https://example.com"),
				WithClientKey("k"),
				WithFeatureCache(cache))
			require.NoError(t, err)
			require.Equal(t, client.EvalFeature(ctx, "feat").Value, seeded.EvalFeature(ctx, "feat").Value)
			require.Equal(t, client.data.savedGroups, seeded.data.savedGroups)
		})
	}
}

// A payload without features is never cached on its own: a client seeded
// from it would start with no features at all.
func TestPayloadWithoutFeaturesIsNotCached(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache))
	require.NoError(t, err)

	require.NoError(t, client.UpdateFromApiResponseJSON(`{"savedGroups":{"beta":["u1"]},"dateUpdated":"2020-01-01T00:00:00Z"}`))
	_, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.False(t, ok)

	// Once features arrive, the earlier groups are cached with them.
	require.NoError(t, client.UpdateFromApiResponseJSON(`{"features":{"feat":{"defaultValue":1}},"dateUpdated":"2020-01-02T00:00:00Z"}`))
	entry, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, `{"features":{"feat":{"defaultValue":1}},"savedGroups":{"beta":["u1"]},"dateUpdated":"2020-01-02T00:00:00Z"}`, string(entry.Payload))
}

// The client rejects a response older than its data; the cache must not take
// it either, or a restarted client would load the rejected features.
func TestRejectedOlderResponseIsNotCached(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache))
	require.NoError(t, err)

	newer := `{"features":{"feat":{"defaultValue":"newer"}},"dateUpdated":"2020-01-02T00:00:00Z"}`
	require.NoError(t, client.UpdateFromApiResponseJSON(newer))
	require.NoError(t, client.UpdateFromApiResponseJSON(`{"features":{"feat":{"defaultValue":"older"}},"dateUpdated":"2020-01-01T00:00:00Z"}`))

	require.Equal(t, "newer", client.EvalFeature(ctx, "feat").Value)
	entry, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, newer, string(entry.Payload))
}

// Plaintext and encrypted keys encode the same section, so an update in one
// form replaces the other instead of caching both.
func TestEncryptedSectionReplacesPlaintextInCache(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithDecryptionKey(savedGroupsTestKey),
		WithFeatureCache(cache))
	require.NoError(t, err)

	require.NoError(t, client.UpdateFromApiResponseJSON(`{"features":{"feat":{"defaultValue":"plain"}},"dateUpdated":"2020-01-01T00:00:00Z"}`))
	encrypted := encryptSavedGroupsTestJSON(t, `{"feat":{"defaultValue":"secret"}}`)
	require.NoError(t, client.UpdateFromApiResponseJSON(`{"encryptedFeatures":"`+encrypted+`","dateUpdated":"2020-01-02T00:00:00Z"}`))

	entry, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, `{"encryptedFeatures":"`+encrypted+`","dateUpdated":"2020-01-02T00:00:00Z"}`, string(entry.Payload))
}

// Features set directly have no raw payload to cache. They are dropped from
// the cache snapshot, so a later partial update cannot cache stale features
// alongside newer sections.
func TestDirectlySetFeaturesStopCacheWrites(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache))
	require.NoError(t, err)

	full := `{"features":{"feat":{"defaultValue":"api"}},"dateUpdated":"2020-01-01T00:00:00Z"}`
	require.NoError(t, client.UpdateFromApiResponseJSON(full))
	require.NoError(t, client.SetFeatures(FeatureMap{"feat": &Feature{DefaultValue: "direct"}}))
	require.NoError(t, client.UpdateFromApiResponseJSON(`{"savedGroups":{"beta":["u1"]},"dateUpdated":"2020-01-02T00:00:00Z"}`))

	entry, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, full, string(entry.Payload))
}

// blockingFeatureCache blocks its first Set until release is closed, so a test
// can hold one cache write in flight while another update runs.
type blockingFeatureCache struct {
	*InMemoryFeatureCache
	inSet   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingFeatureCache) Set(ctx context.Context, key string, entry *FeatureCacheEntry) error {
	first := false
	c.once.Do(func() { first = true })
	if first {
		close(c.inSet)
		<-c.release
	}
	return c.InMemoryFeatureCache.Set(ctx, key, entry)
}

// While one update's cache write is in flight, a newer update must wait for
// it; otherwise the older write could finish last and move the cache backward.
func TestCacheWritesFollowUpdateOrder(t *testing.T) {
	cache := &blockingFeatureCache{
		InMemoryFeatureCache: NewInMemoryFeatureCache(),
		inSet:                make(chan struct{}),
		release:              make(chan struct{}),
	}
	client, err := NewClient(ctx,
		WithApiHost("https://example.com"),
		WithClientKey("k"),
		WithFeatureCache(cache))
	require.NoError(t, err)

	older := `{"features":{"feat":{"defaultValue":"older"}},"dateUpdated":"2020-01-01T00:00:00Z"}`
	newer := `{"features":{"feat":{"defaultValue":"newer"}},"dateUpdated":"2020-01-02T00:00:00Z"}`

	olderDone := make(chan error, 1)
	go func() { olderDone <- client.UpdateFromApiResponseJSON(older) }()
	<-cache.inSet

	newerDone := make(chan error, 1)
	go func() { newerDone <- client.UpdateFromApiResponseJSON(newer) }()

	// The newer update must not complete while the older write is held. This
	// negative check needs a bound; a broken implementation finishes well
	// within it, while a correct one never does.
	select {
	case <-newerDone:
		t.Fatal("newer update finished while an older cache write was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(cache.release)
	require.NoError(t, <-olderDone)
	require.NoError(t, <-newerDone)

	entry, ok, err := cache.Get(ctx, "https://example.com::k")
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, newer, string(entry.Payload))
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

// DateUpdated mirrors the payload's dateUpdated, so a backend can order
// entries without parsing the payload; zero when the data never had one.
func TestWriteThroughRecordsDateUpdated(t *testing.T) {
	written := func(payload string) *FeatureCacheEntry {
		t.Helper()
		cache := NewInMemoryFeatureCache()
		client, err := NewClient(ctx,
			WithApiHost("https://example.com"), WithClientKey("k"), WithFeatureCache(cache))
		require.NoError(t, err)
		require.NoError(t, client.UpdateFromApiResponseJSON(payload))
		entry, ok, err := cache.Get(ctx, "https://example.com::k")
		require.NoError(t, err)
		require.True(t, ok)
		return entry
	}

	dated := written(`{"features":{"feat":{"defaultValue":1}},"dateUpdated":"2020-01-02T00:00:00Z"}`)
	require.Equal(t, time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), dated.DateUpdated.UTC())

	undated := written(`{"features":{"feat":{"defaultValue":1}}}`)
	require.True(t, undated.DateUpdated.IsZero())
}

func TestInMemoryFeatureCacheNeverGoesBack(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2020, 1, d, 0, 0, 0, 0, time.UTC) }
	set := func(cache *InMemoryFeatureCache, etag string, date time.Time) {
		t.Helper()
		require.NoError(t, cache.Set(ctx, "k", &FeatureCacheEntry{
			Payload: json.RawMessage(`{"features":{}}`), Etag: etag, DateUpdated: date,
		}))
	}
	stored := func(cache *InMemoryFeatureCache) string {
		t.Helper()
		got, ok, err := cache.Get(ctx, "k")
		require.NoError(t, err)
		require.True(t, ok)
		return got.Etag
	}

	cache := NewInMemoryFeatureCache()
	set(cache, "newer", day(2))
	set(cache, "older", day(1))
	require.Equal(t, "newer", stored(cache), "an older write is skipped")

	set(cache, "same", day(2))
	require.Equal(t, "same", stored(cache), "an equally dated write replaces, e.g. a new etag")

	set(cache, "undated", time.Time{})
	require.Equal(t, "undated", stored(cache), "a zero date is unknown and always written")
}

// The reviewer's scenario: clients sharing one backend, one of them receiving
// older data after another cached newer. A client started afterwards must
// still seed the newer data.
func TestSharedCacheKeepsNewestAcrossClients(t *testing.T) {
	cache := NewInMemoryFeatureCache()
	newClient := func() *Client {
		t.Helper()
		c, err := NewClient(ctx,
			WithApiHost("https://example.com"), WithClientKey("k"), WithFeatureCache(cache))
		require.NoError(t, err)
		return c
	}

	a, b := newClient(), newClient()
	require.NoError(t, b.UpdateFromApiResponseJSON(
		`{"features":{"feat":{"defaultValue":"newer"}},"dateUpdated":"2020-01-02T00:00:00Z"}`))
	// a has seen nothing newer itself, so it accepts the older data.
	require.NoError(t, a.UpdateFromApiResponseJSON(
		`{"features":{"feat":{"defaultValue":"older"}},"dateUpdated":"2020-01-01T00:00:00Z"}`))
	require.Equal(t, "older", a.EvalFeature(ctx, "feat").Value)

	require.Equal(t, "newer", newClient().EvalFeature(ctx, "feat").Value)
}
