package growthbook

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// RefreshMode controls whether a feature refresh may be skipped because the
// client's features are still fresh.
type RefreshMode int

const (
	// RefreshDefault skips the fetch while the last successful one is newer
	// than the interval set with WithBackgroundFetchInterval.
	RefreshDefault RefreshMode = iota
	// RefreshForce always fetches, bypassing the freshness check, the ETag
	// and HTTP caches.
	RefreshForce
)

const (
	DefaultRetryMaxAttempts  = 5
	DefaultRetryInitialDelay = time.Second
	DefaultRetryMaxDelay     = 16 * time.Second
)

// RetryPolicy configures bounded exponential backoff for feature fetches.
// Network errors and 408, 429 and 5xx responses are retried; other failures
// return at once.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, including the first one.
	// 1 disables retries.
	MaxAttempts int
	// InitialDelay is the wait before the first retry. Each later retry
	// doubles it.
	InitialDelay time.Duration
	// MaxDelay caps any single wait.
	MaxDelay time.Duration
}

// DefaultRetryPolicy returns the policy clients use unless WithRetryPolicy
// sets another: five attempts, with waits of 1s, 2s, 4s and 8s between them.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:  DefaultRetryMaxAttempts,
		InitialDelay: DefaultRetryInitialDelay,
		MaxDelay:     DefaultRetryMaxDelay,
	}
}

func (p RetryPolicy) validate() error {
	if p.MaxAttempts < 1 {
		return errors.New("retry policy: MaxAttempts must be at least 1")
	}
	if p.InitialDelay < 0 {
		return errors.New("retry policy: InitialDelay must not be negative")
	}
	if p.MaxDelay < 0 {
		return errors.New("retry policy: MaxDelay must not be negative")
	}
	return nil
}

// delayBeforeAttempt returns the wait before the one-based attempt.
func (p RetryPolicy) delayBeforeAttempt(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	delay := p.InitialDelay
	for i := 2; i < attempt; i++ {
		if delay > p.MaxDelay/2 {
			return p.MaxDelay
		}
		delay *= 2
	}
	return min(delay, p.MaxDelay)
}

// isRetryableFetchError reports whether a failed fetch may succeed if repeated.
func isRetryableFetchError(err error) bool {
	var apiErr *FeatureApiError
	if errors.As(err, &apiErr) {
		code := apiErr.StatusCode
		return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500
	}
	// url.Error is itself a net.Error, so look at what it wraps: a malformed
	// URL or unsupported scheme won't fix itself.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

var noRetry = RetryPolicy{MaxAttempts: 1}

// fetchFeatures calls the features API under the client's retry policy.
func (client *Client) fetchFeatures(ctx context.Context, etag string, mode RefreshMode) (*FeatureApiResponse, error) {
	return client.fetchFeaturesWith(ctx, etag, mode, client.data.getRetryPolicy())
}

// fetchFeaturesWith calls the features API under policy. A cancelled ctx stops
// the retries and returns its error.
func (client *Client) fetchFeaturesWith(ctx context.Context, etag string, mode RefreshMode, policy RetryPolicy) (*FeatureApiResponse, error) {
	for attempt := 1; ; attempt++ {
		resp, err := client.callFeatureApi(ctx, etag, mode)
		if err == nil || attempt >= policy.MaxAttempts || ctx.Err() != nil || !isRetryableFetchError(err) {
			return resp, err
		}
		delay := policy.delayBeforeAttempt(attempt + 1)
		client.logger.WarnContext(ctx, "Feature fetch failed, retrying",
			"error", err, "attempt", attempt+1, "maxAttempts", policy.MaxAttempts, "delay", delay)
		if err := sleepCtx(ctx, delay); err != nil {
			return resp, err
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// applyFetched applies a fetched response, unless it is a 304, records the
// fetch for the freshness check, and reports the outcome to the refresh
// handler.
func (client *Client) applyFetched(ctx context.Context, source RefreshSource, resp *FeatureApiResponse) error {
	if resp.Status == http.StatusNotModified {
		client.data.markFetched(time.Now())
		client.notifyRefresh(ctx, RefreshResult{Source: source, NotModified: true, DateUpdated: client.data.getDateUpdated()})
		return nil
	}
	// A 200 payload always applies: applyApiResponse preserves omitted
	// sections, so partial responses update just what they carry.
	applied, dateUpdated, err := client.applyApiResponse(resp)
	if err == nil {
		client.data.markFetched(time.Now())
	}
	client.notifyRefreshOutcome(ctx, source, applied, dateUpdated, err)
	return err
}

// RefreshFeaturesWithMode fetches the latest features from the GrowthBook API
// and updates the client, retrying failures under the client's retry policy.
// RefreshDefault skips the fetch while the features are fresher than the
// interval set with WithBackgroundFetchInterval; RefreshForce always fetches.
// The current features stay in use until the new ones are applied.
//
// Concurrent calls with the same mode share one fetch and its result. A
// handler set with WithFeaturesRefreshHandler sees one event per fetch, after
// any retries; a refresh skipped as fresh makes no fetch and reports nothing.
func (client *Client) RefreshFeaturesWithMode(ctx context.Context, mode RefreshMode) error {
	if mode != RefreshForce && client.data.isFresh(time.Now()) {
		client.logger.DebugContext(ctx, "Skipping feature refresh, features are fresher than the background fetch interval")
		return nil
	}
	// Only the call that runs the fetch reports it, so callers sharing one
	// fetch produce one refresh event.
	return client.data.refreshes.do(ctx, mode, func(ctx context.Context) error {
		resp, err := client.fetchFeatures(ctx, "", mode)
		if err != nil {
			client.notifyRefresh(ctx, RefreshResult{Source: RefreshSourceManual, Error: err})
			return err
		}
		return client.applyFetched(ctx, RefreshSourceManual, resp)
	})
}

// refreshGroup runs one refresh per mode at a time; callers that arrive while
// one runs wait for its result instead of fetching again.
type refreshGroup struct {
	mu    sync.Mutex
	calls map[RefreshMode]*refreshCall
}

type refreshCall struct {
	done    chan struct{}
	err     error
	waiters int
}

func (g *refreshGroup) do(ctx context.Context, mode RefreshMode, refresh func(context.Context) error) error {
	for {
		call, leader := g.join(mode)
		if leader {
			call.err = refresh(ctx)
			g.leave(mode)
			close(call.done)
			return call.err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-call.done:
		}
		// The leader's own context ended its refresh, not a failure this
		// caller should share: run it again.
		if isContextError(call.err) {
			continue
		}
		return call.err
	}
}

// join returns the running call for mode, or starts one with the caller as
// its leader.
func (g *refreshGroup) join(mode RefreshMode) (*refreshCall, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if call, ok := g.calls[mode]; ok {
		call.waiters++
		return call, false
	}
	if g.calls == nil {
		g.calls = make(map[RefreshMode]*refreshCall)
	}
	call := &refreshCall{done: make(chan struct{})}
	g.calls[mode] = call
	return call, true
}

func (g *refreshGroup) leave(mode RefreshMode) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.calls, mode)
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// RefreshSource identifies which mechanism produced a refresh event.
type RefreshSource string

const (
	RefreshSourcePoll   RefreshSource = "poll"
	RefreshSourceSSE    RefreshSource = "sse"
	RefreshSourceManual RefreshSource = "manual"
)

// RefreshResult describes the outcome of a single feature refresh attempt.
// Exactly one of Updated, NotModified, or Error describes the outcome:
// Updated when new definitions were applied, NotModified when the stored
// definitions were confirmed still current without being replaced, and Error
// when the attempt failed.
type RefreshResult struct {
	// Source identifies which mechanism produced the event: polling, SSE,
	// or a manual RefreshFeatures call.
	Source RefreshSource
	// Updated is true when a payload was fetched and applied to at least one
	// section (features, saved groups or contextual bandits).
	Updated bool
	// NotModified is true when nothing was replaced: the server returned
	// HTTP 304, the response was refused as older than the current data, or
	// the payload carried no section to apply. The stored definitions remain
	// in effect in all three cases.
	NotModified bool
	// Error is non-nil when the refresh attempt failed: network error,
	// non-2xx/304 status, an undecodable response, a features payload that
	// fails to decrypt, or invalid saved groups. A failure to decrypt
	// EncryptedSavedGroups or EncryptedContextualBandits is not reported here:
	// that section is logged and ignored (the plaintext fallback or the
	// previous definitions stay in effect), and the attempt is classified by
	// the remaining sections as Updated or NotModified.
	Error error
	// DateUpdated is the dateUpdated the client holds after the attempt, never
	// a timestamp it refused: the payload's own value when the payload was
	// processed, and the currently stored value on a 304 or when the response
	// was refused as older than the current data. It is the zero time when
	// Error is set.
	DateUpdated time.Time
}

// FeaturesRefreshHandler is invoked after every feature refresh attempt made
// by a background datasource (polling or SSE) or a manual RefreshFeatures
// call. It also fires on the initial load. Implementations should return
// quickly and must not block the datasource; panics are recovered and logged.
// The handler is shared across child clients created via With* methods -
// register it once on the root client.
//
// The initial-load event runs before the client becomes ready: EnsureLoaded
// does not return until the handler does. The handler must therefore not call
// EnsureLoaded, or anything that waits on it, since startup and the handler
// would wait on each other. The handler is invoked without the client's data
// lock held, but that does not make this call safe.
type FeaturesRefreshHandler func(ctx context.Context, result RefreshResult)

// notifyRefresh safely invokes the configured handler. It reads the handler
// under the lock, then invokes it WITHOUT holding the lock (user code may call
// back into the client), recovering from panics so a handler never breaks the
// datasource loop.
func (client *Client) notifyRefresh(ctx context.Context, r RefreshResult) {
	h := client.data.getRefreshHandler()
	if h == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			client.logger.ErrorContext(ctx, "Refresh handler panicked", "error", rec)
		}
	}()
	h(ctx, r)
}

// notifyRefreshOutcome reports one apply attempt as exactly one of Updated,
// NotModified or Error. Every path that applies a fetched payload - polling,
// SSE initial load, SSE events and manual RefreshFeatures - classifies through
// here, so the three outcomes cannot drift apart between sources, and a
// payload that was not applied reports the timestamp the client kept rather
// than the one it refused.
func (client *Client) notifyRefreshOutcome(ctx context.Context, source RefreshSource, applied bool, dateUpdated time.Time, err error) {
	switch {
	case err != nil:
		client.notifyRefresh(ctx, RefreshResult{Source: source, Error: err})
	case applied:
		client.notifyRefresh(ctx, RefreshResult{Source: source, Updated: true, DateUpdated: dateUpdated})
	default:
		client.notifyRefresh(ctx, RefreshResult{Source: source, NotModified: true, DateUpdated: dateUpdated})
	}
}
