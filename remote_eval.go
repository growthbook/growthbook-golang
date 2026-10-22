package growthbook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// defaultRemoteEvalTimeout bounds a single remote-eval request, so a server that
// accepts the connection but never answers cannot hang evaluations.
const defaultRemoteEvalTimeout = 10 * time.Second

// WithRemoteEval enables remote evaluation: features are evaluated on a remote
// endpoint (POST {apiHost}/api/eval/{clientKey}) for the client's attributes
// instead of locally. Requires a client key and an apiHost other than
// GrowthBook Cloud (a self-hosted GrowthBook Proxy or remote evaluation
// endpoint), and is incompatible with a decryption key or a poll/SSE data
// source. Default false.
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
// per distinct evaluation input), evicting the oldest-fetched entries first. A
// value of 0 disables the bound. Defaults to 1000.
func WithRemoteEvalCacheSize(size int) ClientOption {
	return func(c *Client) error {
		c.remoteEvalCacheSize = size
		return nil
	}
}

// WithRemoteEvalTimeout bounds how long a single remote-eval request may take,
// independently of the caller's context. A value of 0 disables the timeout.
// Defaults to 10s.
func WithRemoteEvalTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) error {
		c.remoteEvalTimeout = timeout
		return nil
	}
}

// WithRemoteEval creates a child client with remote evaluation toggled.
func (c *Client) WithRemoteEval(enabled bool) (*Client, error) {
	return remoteEvalChild(c.cloneWith(WithRemoteEval(enabled)))
}

// WithCacheKeyAttributes creates a child client with updated cache-key attributes.
func (c *Client) WithCacheKeyAttributes(attrs []string) (*Client, error) {
	return remoteEvalChild(c.cloneWith(WithCacheKeyAttributes(attrs)))
}

// remoteEvalChild validates the remote-eval configuration of a freshly cloned
// child, so child clients enforce the same rules as NewClient.
func remoteEvalChild(client *Client, err error) (*Client, error) {
	if err != nil {
		return nil, err
	}
	if client.remoteEval {
		if err := client.validateRemoteEval(); err != nil {
			return nil, err
		}
	}
	return client, nil
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
	if isGrowthBookCloudHost(client.data.apiHost) {
		return ErrRemoteEvalCloudHost
	}
	return nil
}

// isGrowthBookCloudHost reports whether apiHost points at GrowthBook Cloud,
// which serves no remote-eval endpoint. It matters because the default apiHost
// is the Cloud CDN: without this check, enabling remote eval and forgetting
// WithApiHost would pass validation and then fail every request at runtime.
// The JS SDK rejects the same hosts; unlike its /growthbook\.io$/ match, this
// requires a label boundary, so a host such as notgrowthbook.io is allowed.
// An unparsable apiHost is left for the request itself to report.
func isGrowthBookCloudHost(apiHost string) bool {
	u, err := url.Parse(apiHost)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "growthbook.io" || strings.HasSuffix(host, ".growthbook.io")
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

// maybeLoadRemoteEval returns the remote-eval features for the current
// attributes, loading them lazily. Errors are logged, not returned, so
// evaluation is never blocked: a failed load serves the previously cached
// (stale) features, or none, which yields "unknownFeature".
func (client *Client) maybeLoadRemoteEval(ctx context.Context) FeatureMap {
	features, err := client.loadRemoteEval(ctx, false)
	if err != nil {
		client.logger.WarnContext(ctx, "Remote eval load failed", "error", err)
	}
	return features
}

// loadRemoteEval returns the server-evaluated features for the client's
// attributes, fetching them unless a fresh (non-expired) entry is cached or
// force is set. Concurrent loads for the same key share a single request and
// its outcome. The fetched features are returned directly rather than re-read
// from the shared cache, which may already have evicted them. On failure the
// error is returned together with any previously cached (stale) features.
func (client *Client) loadRemoteEval(ctx context.Context, force bool) (FeatureMap, error) {
	key, err := client.remoteEvalCacheKey()
	if err != nil {
		return nil, err
	}
	if !force {
		if features, ok := client.remoteEvalFresh(key); ok {
			return features, nil
		}
	}

	call, leader := client.data.joinRemoteEvalCall(remoteEvalFlightKey(key, force))
	if leader {
		go client.runRemoteEvalCall(ctx, key, force, call)
	}

	select {
	case <-call.done:
		if call.err == nil && call.features != nil {
			return call.features, nil
		}
		return client.remoteEvalCached(key), call.err
	case <-ctx.Done():
		return client.remoteEvalCached(key), ctx.Err()
	}
}

// remoteEvalFlightKey keys the in-flight map. Forced refreshes get their own
// flight so a refresh never joins an ordinary load, whose outcome may predate
// the refresh or come from the cache without any request; concurrent refreshes
// still share one. Cache keys are JSON objects, so the prefix cannot collide.
func remoteEvalFlightKey(key string, force bool) string {
	if force {
		return "refresh:" + key
	}
	return key
}

// fetchRemoteEval performs one remote-eval request, caches its result and
// returns the features now cached for key: the fetched ones, or newer ones a
// later-started request already cached (nil for a response without features).
func (client *Client) fetchRemoteEval(ctx context.Context, key string) (FeatureMap, error) {
	seq := client.data.remoteEvalSeq.Add(1)
	resp, err := client.callEvalApi(ctx)
	if err != nil {
		return nil, err
	}
	if resp.Features == nil {
		// A missing "features" key is a malformed response; don't cache it.
		client.logger.WarnContext(ctx, "Remote eval response contains no features")
		return nil, nil
	}
	return client.data.setRemoteEval(key, resp.Features, seq, client.remoteEvalCacheSize), nil
}

// runRemoteEvalCall performs the fetch for a shared call and publishes its
// outcome. It runs on its own goroutine with a context detached from the
// leader's cancellation, so a leader that gives up does not fail the callers
// waiting on the same fetch; the remote-eval timeout bounds it instead.
func (client *Client) runRemoteEvalCall(ctx context.Context, key string, force bool, call *remoteEvalCall) {
	defer client.data.finishRemoteEvalCall(remoteEvalFlightKey(key, force), call)
	// A fetch for this key may have completed between the caller's freshness
	// check and joining; reuse it instead of fetching again.
	if features, ok := client.remoteEvalFresh(key); ok && !force {
		call.features = features
		return
	}
	fetchCtx := context.WithoutCancel(ctx)
	if client.remoteEvalTimeout > 0 {
		var cancel context.CancelFunc
		fetchCtx, cancel = context.WithTimeout(fetchCtx, client.remoteEvalTimeout)
		defer cancel()
	}
	call.features, call.err = client.fetchRemoteEval(fetchCtx, key)
}

// remoteEvalFresh returns the cached features for key when they have not
// expired.
func (client *Client) remoteEvalFresh(key string) (FeatureMap, bool) {
	entry, ok := client.data.getRemoteEval(key)
	if !ok {
		return nil, false
	}
	if client.remoteEvalTTL > 0 && client.data.now().Sub(entry.fetchedAt) >= client.remoteEvalTTL {
		return nil, false
	}
	return entry.features, true
}

// remoteEvalCached returns the cached features for key regardless of age, or
// nil when none are cached.
func (client *Client) remoteEvalCached(key string) FeatureMap {
	entry, ok := client.data.getRemoteEval(key)
	if !ok {
		return nil
	}
	return entry.features
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
	// Send an empty object rather than null when no forced variations are set,
	// matching the other SDKs' wire shape.
	forcedVariations := client.forcedVariations
	if forcedVariations == nil {
		forcedVariations = ForcedVariationsMap{}
	}
	body, err := json.Marshal(remoteEvalPayload{
		Attributes:       attrs,
		ForcedVariations: forcedVariations,
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

// remoteEvalFeatures returns the cached server-evaluated features for the
// client's current inputs, or nil when they have not been loaded yet.
func (client *Client) remoteEvalFeatures() FeatureMap {
	key, err := client.remoteEvalCacheKey()
	if err != nil {
		return nil
	}
	return client.remoteEvalCached(key)
}
