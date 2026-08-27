package growthbook

import (
	"container/list"
	"sync"
)

// defaultExperimentTrackingCacheSize bounds the experiment_viewed dedup set so a
// long-lived multi-user client does not grow it without limit.
const defaultExperimentTrackingCacheSize = 10000

// trackKey identifies a unique experiment exposure. A typed struct key is
// collision-safe, unlike joining fields into one delimited string.
type trackKey struct {
	hashAttribute string
	hashValue     string
	experimentKey string
	variationID   int
}

// trackedSet is a bounded LRU set recording which experiment assignments have
// already been tracked, so experiment_viewed fires once per unique assignment.
// It is safe for concurrent use and shared across child clients via the client's
// data.
type trackedSet struct {
	mu    sync.Mutex
	max   int
	items map[trackKey]*list.Element
	order *list.List
}

// markOnce records key and reports whether it was newly added (true means the
// assignment has not been tracked before and should be tracked now). When the
// set is at capacity, the least-recently-tracked entry is evicted.
func (t *trackedSet) markOnce(key trackKey) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.items == nil {
		t.items = make(map[trackKey]*list.Element)
		t.order = list.New()
	}
	if elem, ok := t.items[key]; ok {
		t.order.MoveToFront(elem)
		return false
	}
	t.items[key] = t.order.PushFront(key)
	for t.max > 0 && len(t.items) > t.max {
		back := t.order.Back()
		if back == nil {
			break
		}
		delete(t.items, back.Value.(trackKey))
		t.order.Remove(back)
	}
	return true
}

// hasTrackingConsumer reports whether anything actually consumes experiment_viewed
// tracking (a callback or a plugin), so the dedup set is only populated when there
// is a consumer.
func (client *Client) hasTrackingConsumer() bool {
	return client.experimentCallback != nil || len(client.data.getPlugins()) > 0
}

// shouldTrackExperiment reports whether this assignment has not yet been tracked
// for the current (hashAttribute, hashValue, experiment key, variation), marking
// it tracked. It returns false when there is no tracking consumer, so the dedup
// set stays empty for clients that don't track.
func (client *Client) shouldTrackExperiment(exp *Experiment, res *ExperimentResult) bool {
	if !client.hasTrackingConsumer() {
		return false
	}
	return client.data.tracked.markOnce(trackKey{
		hashAttribute: res.HashAttribute,
		hashValue:     res.HashValue,
		experimentKey: exp.Key,
		variationID:   res.VariationId,
	})
}
