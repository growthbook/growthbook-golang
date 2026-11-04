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

// applyFetched applies a fetched response, unless it is a 304, and records
// the fetch for the freshness check.
func (client *Client) applyFetched(resp *FeatureApiResponse) error {
	if resp.Status != http.StatusNotModified {
		if err := client.UpdateFromApiResponse(resp); err != nil {
			return err
		}
	}
	client.data.markFetched(time.Now())
	return nil
}

// RefreshFeaturesWithMode fetches the latest features from the GrowthBook API
// and updates the client, retrying failures under the client's retry policy.
// RefreshDefault skips the fetch while the features are fresher than the
// interval set with WithBackgroundFetchInterval; RefreshForce always fetches.
// The current features stay in use until the new ones are applied.
//
// Concurrent calls with the same mode share one fetch and its result.
func (client *Client) RefreshFeaturesWithMode(ctx context.Context, mode RefreshMode) error {
	if mode != RefreshForce && client.data.isFresh(time.Now()) {
		client.logger.DebugContext(ctx, "Skipping feature refresh, features are fresher than the background fetch interval")
		return nil
	}
	return client.data.refreshes.do(ctx, mode, func(ctx context.Context) error {
		resp, err := client.fetchFeatures(ctx, "", mode)
		if err != nil {
			return err
		}
		return client.applyFetched(resp)
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
