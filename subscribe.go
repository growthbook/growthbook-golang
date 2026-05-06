package growthbook

import (
	"context"
	"sync"
	"sync/atomic"
)

// ExperimentSubscriber is invoked when an experiment assignment changes.
type ExperimentSubscriber func(ctx context.Context, exp *Experiment, result *ExperimentResult)

type subscriberAssignment struct {
	inExperiment bool
	variationID  int
}

type subscriberRegistry struct {
	mu       sync.RWMutex
	nextID   atomic.Uint64
	entries  map[uint64]ExperimentSubscriber
	assigned map[string]subscriberAssignment
}

func (r *subscriberRegistry) add(fn ExperimentSubscriber) func() {
	id := r.nextID.Add(1)
	r.mu.Lock()
	if r.entries == nil {
		r.entries = make(map[uint64]ExperimentSubscriber)
	}
	r.entries[id] = fn
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		delete(r.entries, id)
		r.mu.Unlock()
	}
}

func (r *subscriberRegistry) subscribersForResult(exp *Experiment, res *ExperimentResult) []ExperimentSubscriber {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) == 0 {
		return nil
	}
	if r.assigned == nil {
		r.assigned = make(map[string]subscriberAssignment)
	}
	next := subscriberAssignment{
		inExperiment: res.InExperiment,
		variationID:  res.VariationId,
	}
	prev, ok := r.assigned[exp.Key]
	r.assigned[exp.Key] = next
	if ok && prev == next {
		return nil
	}
	out := make([]ExperimentSubscriber, 0, len(r.entries))
	for _, fn := range r.entries {
		out = append(out, fn)
	}
	return out
}

// Subscribe registers a callback fired when an experiment assignment changes.
// The returned function unregisters it. Subscribers are shared across child
// clients created via With* methods - register once on the root client.
func (client *Client) Subscribe(fn ExperimentSubscriber) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	return client.data.subscribers.add(fn)
}

func (client *Client) notifySubscribers(ctx context.Context, exp *Experiment, res *ExperimentResult) {
	for _, fn := range client.data.subscribers.subscribersForResult(exp, res) {
		client.safeNotifySubscriber(ctx, fn, exp, res)
	}
}

func (client *Client) safeNotifySubscriber(ctx context.Context, fn ExperimentSubscriber, exp *Experiment, res *ExperimentResult) {
	defer func() {
		if r := recover(); r != nil {
			client.logger.ErrorContext(ctx, "Subscriber panicked", "error", r)
		}
	}()
	fn(ctx, exp, res)
}
