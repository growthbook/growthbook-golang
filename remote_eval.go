package growthbook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// defaultRemoteEvalTTL is how long a cached remote-eval result is reused before
// a new request is made, so feature changes on the server are eventually picked
// up. A value of 0 disables expiry.
const defaultRemoteEvalTTL = 60 * time.Second

// defaultRemoteEvalCacheSize bounds the number of cached remote-eval results
// (one per distinct attribute set) so a long-running process with
// high-cardinality attributes does not grow the cache without limit.
const defaultRemoteEvalCacheSize = 1000

// keyedMutex serializes work per key, coalescing concurrent remote fetches for
// the same attributes so they issue a single request (single-flight).
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	if k.m == nil {
		k.m = make(map[string]*sync.Mutex)
	}
	mu, ok := k.m[key]
	if !ok {
		mu = &sync.Mutex{}
		k.m[key] = mu
	}
	k.mu.Unlock()
	mu.Lock()
	return mu.Unlock
}

func (k *keyedMutex) delete(key string) {
	k.mu.Lock()
	delete(k.m, key)
	k.mu.Unlock()
}

// WithRemoteEval enables remote evaluation: features are evaluated on a remote
// endpoint (POST {apiHost}/api/eval/{clientKey}) for the client's attributes
// instead of locally. Requires a client key and is incompatible with a
// decryption key or a poll/SSE data source. Default false.
func WithRemoteEval(enabled bool) ClientOption {
	return func(c *Client) error {
		c.remoteEval = enabled
		return nil
	}
}

// WithCacheKeyAttributes limits which attributes are used to key the remote-eval
// cache: a new remote request is made only when one of these attributes changes.
// When empty, all attributes are used.
func WithCacheKeyAttributes(attrs []string) ClientOption {
	return func(c *Client) error {
		c.cacheKeyAttributes = attrs
		return nil
	}
}

// WithRemoteEvalTTL sets how long a cached remote-eval result is reused before a
// new request is made. A value of 0 disables expiry. Defaults to 60s.
func WithRemoteEvalTTL(ttl time.Duration) ClientOption {
	return func(c *Client) error {
		c.remoteEvalTTL = ttl
		return nil
	}
}

// WithRemoteEvalCacheSize bounds how many remote-eval results are cached (one
// per distinct attribute set), evicting least-recently-used entries. A value of
// 0 disables the bound. Defaults to 1000.
func WithRemoteEvalCacheSize(size int) ClientOption {
	return func(c *Client) error {
		c.remoteEvalCacheSize = size
		return nil
	}
}

// WithRemoteEval creates a child client with remote evaluation toggled.
func (c *Client) WithRemoteEval(enabled bool) (*Client, error) {
	return c.cloneWith(WithRemoteEval(enabled))
}

// WithCacheKeyAttributes creates a child client with updated cache-key attributes.
func (c *Client) WithCacheKeyAttributes(attrs []string) (*Client, error) {
	return c.cloneWith(WithCacheKeyAttributes(attrs))
}

func (client *Client) validateRemoteEval() error {
	if client.data.clientKey == "" {
		return ErrRemoteEvalNoClientKey
	}
	if client.data.decryptionKey != "" {
		return ErrRemoteEvalDecryptionKey
	}
	if client.data.dataSource != nil {
		return ErrRemoteEvalWithDataSource
	}
	return nil
}

// remoteEvalCacheKey returns a stable key covering every input sent to the
// remote-eval endpoint: the attributes, forced variations and URL. Keying on all
// of them ensures a change to any (not just attributes) triggers a fresh
// evaluation instead of reusing a result computed for different inputs. When
// cacheKeyAttributes is set, only those attributes contribute to the key.
func (client *Client) remoteEvalCacheKey() (string, error) {
	attrs := client.attributes
	if len(client.cacheKeyAttributes) > 0 {
		selected := value.ObjValue{}
		for _, k := range client.cacheKeyAttributes {
			if v, ok := attrs[k]; ok {
				selected[k] = v
			}
		}
		attrs = selected
	}
	pageURL := ""
	if client.url != nil {
		pageURL = client.url.String()
	}
	// json.Marshal sorts map keys, so the key is deterministic.
	b, err := json.Marshal(struct {
		Attributes       value.ObjValue      `json:"attributes"`
		ForcedVariations ForcedVariationsMap `json:"forcedVariations"`
		URL              string              `json:"url"`
	}{attrs, client.forcedVariations, pageURL})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// maybeLoadRemoteEval lazily loads the remote-eval features for the current
// attributes when remote eval is enabled. Errors are logged, not returned, so
// evaluation is never blocked (a failed load yields "unknownFeature", matching
// the documented remote-eval fallback behavior).
func (client *Client) maybeLoadRemoteEval(ctx context.Context) {
	if !client.remoteEval {
		return
	}
	if err := client.loadRemoteEval(ctx, false); err != nil {
		client.logger.WarnContext(ctx, "Remote eval load failed", "error", err)
	}
}

// loadRemoteEval fetches server-evaluated features for the client's attributes
// and caches them. When force is false and a fresh (non-expired) entry is
// cached, it is a no-op. Concurrent loads for the same key are coalesced into a
// single request. On fetch failure any previously cached entry is kept, so a
// stale result is still served rather than nothing.
func (client *Client) loadRemoteEval(ctx context.Context, force bool) error {
	key, err := client.remoteEvalCacheKey()
	if err != nil {
		return err
	}
	if !force && client.remoteEvalFresh(key) {
		return nil
	}

	unlock := client.data.remoteEvalFlight.lock(key)
	defer unlock()
	// Another goroutine may have refreshed this key while we waited.
	if !force && client.remoteEvalFresh(key) {
		return nil
	}

	resp, err := client.callEvalApi(ctx)
	if err != nil {
		return err
	}
	if resp.Features == nil {
		// A missing "features" key is a malformed response; don't cache it.
		client.logger.WarnContext(ctx, "Remote eval response contains no features")
		return nil
	}
	client.data.setRemoteEval(key, resp.Features, client.remoteEvalCacheSize)
	return nil
}

// remoteEvalFresh reports whether a non-expired cache entry exists for key.
func (client *Client) remoteEvalFresh(key string) bool {
	entry, ok := client.data.getRemoteEval(key)
	if !ok {
		return false
	}
	if client.remoteEvalTTL <= 0 {
		return true
	}
	return client.data.now().Sub(entry.fetchedAt) < client.remoteEvalTTL
}

type remoteEvalPayload struct {
	Attributes       json.RawMessage     `json:"attributes"`
	ForcedVariations ForcedVariationsMap `json:"forcedVariations"`
	ForcedFeatures   [][]any             `json:"forcedFeatures"`
	URL              string              `json:"url"`
}

// callEvalApi POSTs the current attributes to the remote-eval endpoint and
// returns the server-evaluated feature response.
func (client *Client) callEvalApi(ctx context.Context) (*FeatureApiResponse, error) {
	attrs, err := json.Marshal(client.attributes)
	if err != nil {
		return nil, err
	}
	pageURL := ""
	if client.url != nil {
		pageURL = client.url.String()
	}
	body, err := json.Marshal(remoteEvalPayload{
		Attributes:       attrs,
		ForcedVariations: client.forcedVariations,
		ForcedFeatures:   [][]any{},
		URL:              pageURL,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.data.getEvalUrl(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.data.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote eval failed, code: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	apiResp := FeatureApiResponse{}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		client.logger.ErrorContext(ctx, "Error parsing remote eval response", "error", err)
		return nil, err
	}
	return &apiResp, nil
}
