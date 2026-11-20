package growthbook

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

// FeatureRefreshSubscriber is invoked when a new feature payload has been applied.
//
// Each subscriber receives its own response value whose maps are detached from the client's live
// state, so mutating it cannot change later evaluations, race with an evaluation in progress, or
// reach another subscriber. The values inside those maps (*Feature, saved groups, bandit
// definitions) are shared and must be treated as read-only.
//
// The response is passed as received: encrypted sections stay encrypted, so Features is nil when
// the payload carried EncryptedFeatures.
type FeatureRefreshSubscriber func(ctx context.Context, resp *FeatureApiResponse)

type refreshSubscriberRegistry struct {
	mu      sync.RWMutex
	nextID  atomic.Uint64
	entries map[uint64]FeatureRefreshSubscriber

	// queue holds notifications in the order their payloads changed the data. They are queued
	// while the data lock is held, so two concurrent updates can never be delivered in the
	// opposite order to the one they were applied in, and delivered after it is released.
	queue []queuedRefresh
	// While the data source is starting, notifications stay queued and are delivered once startup
	// has been published, so a subscriber can call EnsureLoaded without waiting on its own callback.
	holding bool
	// delivering is set while one goroutine drains the queue. Only one drains at a time, so
	// notifications are delivered in queue order.
	delivering bool
}

type queuedRefresh struct {
	ctx  context.Context
	resp *FeatureApiResponse
}

func (r *refreshSubscriberRegistry) add(fn FeatureRefreshSubscriber) func() {
	id := r.nextID.Add(1)
	r.mu.Lock()
	if r.entries == nil {
		r.entries = make(map[uint64]FeatureRefreshSubscriber)
	}
	r.entries[id] = fn
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		delete(r.entries, id)
		r.mu.Unlock()
	}
}

func (r *refreshSubscriberRegistry) subscribers() []FeatureRefreshSubscriber {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return nil
	}
	fns := make([]FeatureRefreshSubscriber, 0, len(r.entries))
	for _, fn := range r.entries {
		fns = append(fns, fn)
	}
	return fns
}

func (r *refreshSubscriberRegistry) holdUntilStarted() {
	r.mu.Lock()
	r.holding = true
	r.mu.Unlock()
}

// enqueue queues a notification for an applied payload. The caller holds the data lock, which
// fixes the queue order to the order the data changed; this lock is never held while taking it.
func (r *refreshSubscriberRegistry) enqueue(ctx context.Context, resp *FeatureApiResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Nobody to tell. While starting, the payload is still queued: a subscriber registered before
	// startup finishes receives the initial load.
	if !r.holding && len(r.entries) == 0 {
		return
	}
	// Detached now rather than at delivery: the caller may reuse resp for its next update before
	// this one is delivered, and the notification has to carry this payload, not that one.
	r.queue = append(r.queue, queuedRefresh{ctx: ctx, resp: detachResponse(resp)})
}

// release stops holding notifications once startup has been published.
func (r *refreshSubscriberRegistry) release() {
	r.mu.Lock()
	r.holding = false
	r.mu.Unlock()
}

// startDelivering reports whether the caller should drain the queue, claiming it if so. Nothing
// is delivered while starting or while another goroutine is already draining.
func (r *refreshSubscriberRegistry) startDelivering() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.holding || r.delivering || len(r.queue) == 0 {
		return false
	}
	r.delivering = true
	return true
}

// next pops the oldest queued notification, and gives up the drain once the queue is empty.
func (r *refreshSubscriberRegistry) next() (queuedRefresh, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		r.delivering = false
		return queuedRefresh{}, false
	}
	n := r.queue[0]
	r.queue[0] = queuedRefresh{}
	r.queue = r.queue[1:]
	return n, true
}

// SubscribeFeatureRefresh registers fn to be called whenever a new feature payload is applied, and
// returns a function that removes it again. Evaluation already sees the new payload when fn runs.
//
// A payload applied while the data source is still starting - the initial load included - is
// delivered once startup has finished, so fn may call EnsureLoaded.
func (client *Client) SubscribeFeatureRefresh(fn FeatureRefreshSubscriber) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	return client.data.refreshSubscribers.add(fn)
}

// deliverFeatureRefreshes delivers queued notifications in order, outside the data lock. A payload
// applied while another goroutine is delivering is delivered by that goroutine, behind the ones
// queued before it; so is one applied by a subscriber, which therefore cannot deadlock.
func (client *Client) deliverFeatureRefreshes() {
	r := &client.data.refreshSubscribers
	if !r.startDelivering() {
		return
	}
	for n, ok := r.next(); ok; n, ok = r.next() {
		client.deliverFeatureRefresh(n.ctx, n.resp)
	}
}

// deliverHeldFeatureRefreshes delivers the notifications held while the data source was starting.
// It runs after dsStartWait is closed.
func (client *Client) deliverHeldFeatureRefreshes() {
	client.data.refreshSubscribers.release()
	client.deliverFeatureRefreshes()
}

func (client *Client) deliverFeatureRefresh(ctx context.Context, resp *FeatureApiResponse) {
	for _, fn := range client.data.refreshSubscribers.subscribers() {
		client.safeNotifyRefreshSubscriber(ctx, fn, detachResponse(resp))
	}
}

// detachResponse copies the maps a response carries. The plain payload's maps are stored as the
// client's live state, so handing the response itself to a subscriber would let it write straight
// into what evaluation reads - a data race the runtime kills the process for, not an exception.
func detachResponse(resp *FeatureApiResponse) *FeatureApiResponse {
	if resp == nil {
		return nil
	}

	detached := *resp

	if resp.Features != nil {
		features := make(FeatureMap, len(resp.Features))
		for key, feature := range resp.Features {
			features[key] = feature
		}
		detached.Features = features
	}

	if resp.SavedGroups != nil {
		savedGroups := make(condition.SavedGroups, len(resp.SavedGroups))
		for key, group := range resp.SavedGroups {
			savedGroups[key] = group
		}
		detached.SavedGroups = savedGroups
	}

	if resp.ContextualBandits != nil {
		bandits := make(ContextualBanditDefinitions, len(resp.ContextualBandits))
		for key, bandit := range resp.ContextualBandits {
			bandits[key] = bandit
		}
		detached.ContextualBandits = bandits
	}

	return &detached
}

func (client *Client) safeNotifyRefreshSubscriber(ctx context.Context, fn FeatureRefreshSubscriber, resp *FeatureApiResponse) {
	defer func() {
		if r := recover(); r != nil {
			client.logger.ErrorContext(ctx, "Feature refresh subscriber panicked", "error", r)
		}
	}()
	fn(ctx, resp)
}
