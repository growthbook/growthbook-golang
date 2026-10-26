package growthbook

import (
	"encoding/json"
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
	// payloadJSON is the decrypted payload as JSON values, when the features
	// came from JSON. ReducePayload reads it.
	payloadJSON   map[string]any
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

	// savedGroupsFromCode and banditsFromCode mark sections set from Go
	// values, which payloadJSON doesn't hold
	savedGroupsFromCode bool
	banditsFromCode     bool
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

// getReducibleJSON returns the payload JSON that ReducePayload reduces: the
// stored JSON, with the saved groups and contextual bandits set from Go values
// in place of its sections, since evaluation uses those. It's nil when the
// features were set from Go values.
func (d *data) getReducibleJSON() (map[string]any, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.payloadJSON == nil {
		return nil, nil
	}
	payload := make(map[string]any, len(d.payloadJSON))
	for k, v := range d.payloadJSON {
		payload[k] = v
	}
	for _, s := range []struct {
		key      string
		fromCode bool
		value    any
		isNil    bool
	}{
		{"savedGroups", d.savedGroupsFromCode, d.savedGroups, d.savedGroups == nil},
		{"contextualBandits", d.banditsFromCode, d.contextualBandits, d.contextualBandits == nil},
	} {
		if !s.fromCode {
			continue
		}
		if s.isNil {
			delete(payload, s.key)
			continue
		}
		v, err := jsonValue(s.value)
		if err != nil {
			return nil, err
		}
		payload[s.key] = v
	}
	return payload, nil
}

// jsonValue converts a Go value to its decoded JSON form.
func jsonValue(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	err = json.Unmarshal(data, &out)
	return out, err
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
