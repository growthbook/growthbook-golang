package growthbook

import (
	"strconv"
	"sync"
)

// trackedSet records which experiment assignments have already been tracked so
// experiment_viewed tracking fires only once per unique assignment. It is safe
// for concurrent use and shared across child clients via the client's data.
type trackedSet struct {
	mu sync.Mutex
	m  map[string]struct{}
}

// markOnce records key and reports whether it was newly added (true means the
// assignment has not been tracked before and should be tracked now).
func (t *trackedSet) markOnce(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = make(map[string]struct{})
	}
	if _, ok := t.m[key]; ok {
		return false
	}
	t.m[key] = struct{}{}
	return true
}

// clear forgets all tracked assignments. Called when features change so a new
// configuration can re-emit experiment_viewed tracking.
func (t *trackedSet) clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m = nil
}

// shouldTrackExperiment reports whether this experiment assignment has not yet
// been tracked for the current (hashAttribute, hashValue, experiment key,
// variation), marking it tracked. Used to deduplicate experiment_viewed
// tracking across repeated evaluations of the same assignment.
func (client *Client) shouldTrackExperiment(exp *Experiment, res *ExperimentResult) bool {
	key := res.HashAttribute + "::" + res.HashValue + "::" + exp.Key + "::" + strconv.Itoa(res.VariationId)
	return client.data.tracked.markOnce(key)
}
