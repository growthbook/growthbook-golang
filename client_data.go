package growthbook

import (
	"container/list"
	"net/http"
	"sync"
	"time"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

type data struct {
	mu            sync.RWMutex
	features      FeatureMap
	savedGroups   condition.SavedGroups
	dateUpdated   time.Time
	apiHost       string
	clientKey     string
	decryptionKey string
	httpClient    *http.Client
	dataSource    DataSource
	dsStarted     bool
	dsStartWait   chan struct{}
	dsStartErr    error
	plugins       []Plugin
	subscribers   subscriberRegistry

	// remoteEvalCache maps a remote-eval cache key (derived from the relevant
	// attributes) to its list element; remoteEvalOrder tracks recency so the
	// cache can be bounded with LRU eviction. Shared across child clients so
	// requests with the same attributes reuse one remote fetch.
	remoteEvalCache map[string]*list.Element
	remoteEvalOrder *list.List
	// remoteEvalFlight coalesces concurrent remote fetches for the same key.
	remoteEvalFlight keyedMutex
	// now returns the current time; overridable in tests for TTL checks.
	now func() time.Time
}

// remoteEvalEntry is a cached server-evaluated feature set with its fetch time.
type remoteEvalEntry struct {
	key       string
	features  FeatureMap
	fetchedAt time.Time
}

func newData() *data {
	return &data{
		dsStartWait: make(chan struct{}),
		apiHost:     defaultApiHost,
		httpClient:  http.DefaultClient,
		now:         time.Now,
	}
}

func (d *data) getDateUpdated() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.dateUpdated
}

func (d *data) getFeatures() FeatureMap {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.features
}

func (d *data) getApiUrl() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.apiHost + "/api/features/" + d.clientKey
}

func (d *data) getSseUrl() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.apiHost + "/sub/" + d.clientKey
}

func (d *data) getEvalUrl() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.apiHost + "/api/eval/" + d.clientKey
}

func (d *data) getRemoteEval(key string) (*remoteEvalEntry, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	elem, ok := d.remoteEvalCache[key]
	if !ok {
		return nil, false
	}
	return elem.Value.(*remoteEvalEntry), true
}

// setRemoteEval stores features under key, refreshing recency. When maxSize > 0
// the least-recently-used entries are evicted to keep the cache bounded.
func (d *data) setRemoteEval(key string, features FeatureMap, maxSize int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.remoteEvalCache == nil {
		d.remoteEvalCache = make(map[string]*list.Element)
		d.remoteEvalOrder = list.New()
	}
	if elem, ok := d.remoteEvalCache[key]; ok {
		// Replace the pointer instead of mutating in place: getRemoteEval hands
		// the entry to callers that read it after releasing the RLock, so the
		// entry must be an immutable snapshot.
		elem.Value = &remoteEvalEntry{key: key, features: features, fetchedAt: d.now()}
		d.remoteEvalOrder.MoveToFront(elem)
		return
	}
	elem := d.remoteEvalOrder.PushFront(&remoteEvalEntry{key: key, features: features, fetchedAt: d.now()})
	d.remoteEvalCache[key] = elem

	for maxSize > 0 && len(d.remoteEvalCache) > maxSize {
		back := d.remoteEvalOrder.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*remoteEvalEntry)
		d.remoteEvalOrder.Remove(back)
		delete(d.remoteEvalCache, evicted.key)
		d.remoteEvalFlight.delete(evicted.key)
	}
}

func (d *data) getDsStartErr() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.dsStartErr
}

func (d *data) getDsStarted() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.dsStarted
}

func (d *data) getPlugins() []Plugin {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.plugins
}

func (d *data) getClientKey() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.clientKey
}

type dataUpdate func(*data) error

func (d *data) withLock(f dataUpdate) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return f(d)
}

func (d *data) decrypt(encrypted string) (string, error) {
	d.mu.RLock()
	key := d.decryptionKey
	d.mu.RUnlock()
	if key == "" {
		return "", ErrNoDecryptionKey
	}
	return decrypt(encrypted, key)
}
