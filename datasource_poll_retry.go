package growthbook

import (
	"math"
	"time"
)

const (
	maxRetryAttempts = 5

	baseRetryDelay = 1 * time.Second
	maxRetryDelay  = 16 * time.Second
)

// retryDelay returns how long to wait before the next fetch, given how many consecutive fetches have
// failed. The first maxRetryAttempts failures back off 1s, 2s, 4s, 8s, 16s; beyond that the retries are
// exhausted and the poller waits its configured interval, which is also what a healthy poller waits.
//
// The bound is per cycle rather than per client: a sustained outage settles back into ordinary polling
// instead of either hammering the API or backing off until the data is badly stale.
func retryDelay(interval time.Duration, consecutiveFailures int) time.Duration {
	if consecutiveFailures <= 0 || consecutiveFailures > maxRetryAttempts {
		return interval
	}

	delay := time.Duration(float64(baseRetryDelay) * math.Pow(2, float64(consecutiveFailures-1)))

	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}

	if delay > interval {
		return interval
	}

	return delay
}
