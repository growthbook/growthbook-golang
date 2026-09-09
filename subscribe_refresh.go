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
// reach another subscriber. The *Feature values inside the map are shared and must be treated as
// read-only.
type FeatureRefreshSubscriber func(ctx context.Context, resp *FeatureApiResponse)

type refreshSubscriberRegistry struct {
	mu      sync.RWMutex
	nextID  atomic.Uint64
	entries map[uint64]FeatureRefreshSubscriber
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

// SubscribeFeatureRefresh registers fn to be called whenever a new feature payload is applied, and
// returns a function that removes it again. Evaluation already sees the new payload when fn runs.
func (client *Client) SubscribeFeatureRefresh(fn FeatureRefreshSubscriber) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	return client.data.refreshSubscribers.add(fn)
}

func (client *Client) notifyFeatureRefreshSubscribers(ctx context.Context, resp *FeatureApiResponse) {
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
