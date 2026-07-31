package growthbook

import (
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
	// attributes) to the server-evaluated feature set. Shared across child
	// clients so requests with the same attributes reuse one remote fetch.
	remoteEvalCache map[string]FeatureMap
}

func newData() *data {
	return &data{
		dsStartWait: make(chan struct{}),
		apiHost:     defaultApiHost,
		httpClient:  http.DefaultClient,
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

func (d *data) getRemoteEval(key string) (FeatureMap, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	f, ok := d.remoteEvalCache[key]
	return f, ok
}

func (d *data) setRemoteEval(key string, features FeatureMap) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.remoteEvalCache == nil {
		d.remoteEvalCache = make(map[string]FeatureMap)
	}
	d.remoteEvalCache[key] = features
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
