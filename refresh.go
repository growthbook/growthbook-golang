package growthbook

import (
	"context"
	"time"
)

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
	// Error is non-nil when the refresh attempt failed (network error,
	// non-2xx/304 status, or decode/decrypt failure).
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
