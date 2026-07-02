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
// Updated when new definitions were applied, NotModified when the server
// confirmed the cached features are still current (HTTP 304), and Error
// when the attempt failed.
type RefreshResult struct {
	// Source identifies which mechanism produced the event: polling, SSE,
	// or a manual RefreshFeatures call.
	Source RefreshSource
	// Updated is true when new feature definitions were fetched and applied.
	// It is false when the response was refused as older than current data.
	Updated bool
	// NotModified is true when the server returned HTTP 304, confirming the
	// cached features are still valid without sending a new payload.
	NotModified bool
	// Error is non-nil when the refresh attempt failed (network error,
	// non-2xx/304 status, or decode/decrypt failure).
	Error error
	// DateUpdated is the dateUpdated of the applied payload, or the currently
	// stored value on a 304. It is the zero time when Error is set.
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
