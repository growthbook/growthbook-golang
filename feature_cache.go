package growthbook

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// FeatureCacheEntry is a snapshot of feature data persisted by a FeatureCache.
//
// Payload holds the raw GrowthBook feature API response JSON rather than parsed
// structures. This keeps the entry trivially serializable for external backends
// (e.g. Redis) and lets it round-trip without loss: parsed conditions do not
// marshal back to JSON, so storing them would silently drop targeting rules.
// Storing the raw payload also avoids aliasing the client's live feature map.
type FeatureCacheEntry struct {
	// Payload is the raw feature API response JSON (may contain encrypted features).
	Payload json.RawMessage
	// Etag is the HTTP ETag for the payload, enabling conditional requests after a restart.
	Etag string
	// UpdatedAt is when the SDK wrote this entry, in UTC. Backends without their
	// own expiry — a file, say — need it to age entries out, and it is what a
	// stale-while-revalidate policy would be built on.
	UpdatedAt time.Time
}

// FeatureCache is a pluggable backend for persisting feature data so it can
// survive restarts or be shared across instances (for example, a Redis-backed
// implementation). When configured via WithFeatureCache, the client seeds
// features from the cache on startup and writes back after every successful
// update.
//
// Implementations must be safe for concurrent use. Expiry (TTL) is the
// backend's responsibility; the SDK's data source already governs how often
// features are refreshed from the API.
//
// Both methods return a backend error so failures (e.g. a Redis outage) are
// distinct from a cache miss. The SDK never fails evaluation on a cache error:
// a failed Get on startup is logged and the client continues without a seed,
// and a failed Set is logged.
type FeatureCache interface {
	// Get returns the cached entry for key. found is false on a miss; a non-nil
	// error signals a backend failure (distinct from a miss).
	Get(ctx context.Context, key string) (entry *FeatureCacheEntry, found bool, err error)
	// Set stores entry under key, returning any backend error.
	Set(ctx context.Context, key string, entry *FeatureCacheEntry) error
}

// InMemoryFeatureCache is a thread-safe, process-local FeatureCache. It is the
// default reference implementation; for sharing state across instances supply
// your own (for example, Redis-backed) implementation of FeatureCache.
type InMemoryFeatureCache struct {
	mu    sync.RWMutex
	store map[string]*FeatureCacheEntry
}

// NewInMemoryFeatureCache returns an empty in-memory FeatureCache.
func NewInMemoryFeatureCache() *InMemoryFeatureCache {
	return &InMemoryFeatureCache{store: make(map[string]*FeatureCacheEntry)}
}

func (c *InMemoryFeatureCache) Get(_ context.Context, key string) (*FeatureCacheEntry, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.store[key]
	return entry, ok, nil
}

func (c *InMemoryFeatureCache) Set(_ context.Context, key string, entry *FeatureCacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = entry
	return nil
}
