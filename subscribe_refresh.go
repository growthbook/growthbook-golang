package growthbook

import (
	"context"
	"sync"
	"sync/atomic"
)

// FeatureRefreshSubscriber is invoked when a new feature payload has been applied.
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
		client.safeNotifyRefreshSubscriber(ctx, fn, resp)
	}
}

func (client *Client) safeNotifyRefreshSubscriber(ctx context.Context, fn FeatureRefreshSubscriber, resp *FeatureApiResponse) {
	defer func() {
		if r := recover(); r != nil {
			client.logger.ErrorContext(ctx, "Feature refresh subscriber panicked", "error", r)
		}
	}()
	fn(ctx, resp)
}
