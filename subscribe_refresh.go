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

	// While the data source is starting, notifications are held here and delivered once startup has
	// been published, so a subscriber can call EnsureLoaded without waiting on its own callback.
	holding bool
	held    []heldRefresh
}

type heldRefresh struct {
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

// hold queues a notification while the data source is starting, and reports whether it did.
func (r *refreshSubscriberRegistry) hold(ctx context.Context, resp *FeatureApiResponse) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.holding {
		return false
	}
	// Detached now rather than at delivery: the caller may reuse resp for its next update before
	// startup finishes, and the queued notification has to carry this payload, not that one.
	r.held = append(r.held, heldRefresh{ctx: ctx, resp: detachResponse(resp)})
	return true
}

// takeHeld returns the queued notifications, and stops holding once the queue is empty. Holding
// continues while a batch is delivered, so a payload applied meanwhile is queued behind it rather
// than overtaking it.
func (r *refreshSubscriberRegistry) takeHeld() []heldRefresh {
	r.mu.Lock()
	defer r.mu.Unlock()
	held := r.held
	r.held = nil
	if len(held) == 0 {
		r.holding = false
	}
	return held
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

func (client *Client) notifyFeatureRefreshSubscribers(ctx context.Context, resp *FeatureApiResponse) {
	if client.data.refreshSubscribers.hold(ctx, resp) {
		return
	}
	client.deliverFeatureRefresh(ctx, resp)
}

// deliverHeldFeatureRefreshes delivers the notifications held while the data source was starting.
// It runs after dsStartWait is closed.
func (client *Client) deliverHeldFeatureRefreshes() {
	for held := client.data.refreshSubscribers.takeHeld(); len(held) > 0; held = client.data.refreshSubscribers.takeHeld() {
		for _, h := range held {
			client.deliverFeatureRefresh(h.ctx, h.resp)
		}
	}
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
