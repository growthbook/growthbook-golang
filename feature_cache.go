package growthbook

import (
	"context"
	"encoding/json"
	"slices"
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
	// Payload is feature API response JSON (may contain encrypted sections). It
	// merges the sections of every update the client accepted, so a partial
	// response never drops a section an earlier one supplied.
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
//
// A client writes its updates in order, but clients sharing one backend each
// write their own snapshot, so the last writer wins; the SDK does not
// coordinate writers across instances.
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

// Get returns a copy of the stored entry, so callers cannot change the cached
// snapshot.
func (c *InMemoryFeatureCache) Get(_ context.Context, key string) (*FeatureCacheEntry, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.store[key]
	return cloneFeatureCacheEntry(entry), ok, nil
}

// Set stores a copy of entry, so later changes to it do not reach the cache.
func (c *InMemoryFeatureCache) Set(_ context.Context, key string, entry *FeatureCacheEntry) error {
	clone := cloneFeatureCacheEntry(entry)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = clone
	return nil
}

func cloneFeatureCacheEntry(entry *FeatureCacheEntry) *FeatureCacheEntry {
	if entry == nil {
		return nil
	}
	clone := *entry
	clone.Payload = slices.Clone(entry.Payload)
	return &clone
}

// cacheSections pairs each payload section's plaintext and encrypted keys. The
// two are alternative encodings of one section, so they are replaced together.
var cacheSections = [...]struct{ plain, encrypted string }{
	{"features", "encryptedFeatures"},
	{"savedGroups", "encryptedSavedGroups"},
	{"contextualBandits", "encryptedContextualBandits"},
}

// appliedSections reports which payload sections an update applied.
type appliedSections struct {
	features, savedGroups, bandits bool
}

// mergeCacheSnapshot overlays the sections an update applied onto snapshot,
// mirroring how the client applied them: an applied section replaces both of
// its keys with those in fields, and an unapplied one keeps the snapshot's.
// A nil fields means the update had no raw payload; its applied sections are
// dropped, since the snapshot can no longer reproduce them.
func mergeCacheSnapshot(snapshot, fields map[string]json.RawMessage, applied appliedSections) map[string]json.RawMessage {
	if snapshot == nil {
		snapshot = make(map[string]json.RawMessage)
	}
	flags := [...]bool{applied.features, applied.savedGroups, applied.bandits}
	for i, s := range cacheSections {
		if !flags[i] {
			continue
		}
		for _, key := range [...]string{s.plain, s.encrypted} {
			if v, ok := fields[key]; ok {
				snapshot[key] = v
			} else {
				delete(snapshot, key)
			}
		}
	}
	if fields != nil {
		if v, ok := fields["dateUpdated"]; ok {
			snapshot["dateUpdated"] = v
		} else {
			delete(snapshot, "dateUpdated")
		}
	}
	return snapshot
}

// snapshotHasFeatures reports whether snapshot holds a features section. A
// snapshot without one is not written: a client seeded from it would start
// with no features at all.
func snapshotHasFeatures(snapshot map[string]json.RawMessage) bool {
	_, plain := snapshot["features"]
	_, encrypted := snapshot["encryptedFeatures"]
	return plain || encrypted
}
