package growthbook

import (
	"context"
	"sync"
	"time"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

// FeatureCacheEntry is a snapshot of feature data persisted by a FeatureCache.
type FeatureCacheEntry struct {
	Features    FeatureMap
	SavedGroups condition.SavedGroups
	DateUpdated time.Time
	Etag        string
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
type FeatureCache interface {
	// Get returns the cached entry for key. ok is false on a miss.
	Get(ctx context.Context, key string) (entry *FeatureCacheEntry, ok bool)
	// Set stores entry under key.
	Set(ctx context.Context, key string, entry *FeatureCacheEntry)
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

func (c *InMemoryFeatureCache) Get(_ context.Context, key string) (*FeatureCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.store[key]
	return entry, ok
}

func (c *InMemoryFeatureCache) Set(_ context.Context, key string, entry *FeatureCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = entry
}
