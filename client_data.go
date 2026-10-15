package growthbook

import (
	"container/list"
	"net/http"
	"sync"
	"time"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

type data struct {
	mu                sync.RWMutex
	features          FeatureMap
	savedGroups       condition.SavedGroups
	contextualBandits ContextualBanditDefinitions
	dateUpdated       time.Time
	apiHost           string
	clientKey         string
	decryptionKey     string
	httpClient        *http.Client
	dataSource        DataSource
	dsStarted         bool
	dsStartWait       chan struct{}
	dsStartErr        error
	plugins           []Plugin
	subscribers       subscriberRegistry

	// remoteEvalCache maps a remote-eval cache key (derived from the relevant
	// attributes) to its list element; remoteEvalOrder orders entries by fetch
	// time so the cache can be bounded by evicting the oldest-fetched entries.
	// Shared across child clients so requests with the same attributes reuse
	// one remote fetch.
	remoteEvalCache map[string]*list.Element
	remoteEvalOrder *list.List
	// remoteEvalCalls holds the in-flight fetch per key, so concurrent callers
	// share one request and its outcome, including a failure.
	remoteEvalCallsMu sync.Mutex
	remoteEvalCalls   map[string]*remoteEvalCall

	// now returns the current time; overridable in tests for TTL checks.
	now func() time.Time
}

// remoteEvalCall is a remote-eval fetch shared by every caller that asks for
// the same key while it is in flight. done is closed once features and err are
// set; they are read-only afterwards.
type remoteEvalCall struct {
	done     chan struct{}
	features FeatureMap
	err      error
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

// setRemoteEval stores features under key as the most recently fetched entry.
// When maxSize > 0 the oldest-fetched entries are evicted to keep the cache
// bounded. Reads do not affect eviction order.
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

// joinRemoteEvalCall returns the in-flight call for key, or starts a new one.
// leader is true for the caller that must perform the fetch.
func (d *data) joinRemoteEvalCall(key string) (call *remoteEvalCall, leader bool) {
	d.remoteEvalCallsMu.Lock()
	defer d.remoteEvalCallsMu.Unlock()
	if call, ok := d.remoteEvalCalls[key]; ok {
		return call, false
	}
	if d.remoteEvalCalls == nil {
		d.remoteEvalCalls = make(map[string]*remoteEvalCall)
	}
	call = &remoteEvalCall{done: make(chan struct{})}
	d.remoteEvalCalls[key] = call
	return call, true
}

// finishRemoteEvalCall publishes the call's outcome to its waiters and lets the
// next caller for key start a fresh fetch.
func (d *data) finishRemoteEvalCall(key string, call *remoteEvalCall) {
	d.remoteEvalCallsMu.Lock()
	delete(d.remoteEvalCalls, key)
	d.remoteEvalCallsMu.Unlock()
	close(call.done)
}
