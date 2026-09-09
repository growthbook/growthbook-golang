package growthbook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type PollDataSource struct {
	client   *Client
	logger   *slog.Logger
	interval time.Duration
	failures int
	cancel   context.CancelFunc
	ready    bool
	etag     string
	mu       sync.RWMutex
}

func WithPollDataSource(interval time.Duration) ClientOption {
	return func(c *Client) error {
		if interval <= 0 {
			return fmt.Errorf("growthbook: poll interval must be positive, got %v", interval)
		}

		c.data.dataSource = newPollDataSource(c, interval)
		return nil
	}
}

func newPollDataSource(client *Client, interval time.Duration) *PollDataSource {
	return &PollDataSource{
		client:   client,
		interval: interval,
		logger:   client.logger.With("source", "Growthbook polling datasource"),
	}
}

func (ds *PollDataSource) Start(ctx context.Context) error {
	ds.logger.InfoContext(ctx, "Starting")

	ctx, cancel := context.WithCancel(ctx)
	ds.cancel = cancel

	err := ds.loadData(ctx)

	if err == nil {
		ds.logger.InfoContext(ctx, "First load finished")
	} else {
		ds.recordOutcome(ctx, false)
	}

	ds.mu.Lock()
	ds.ready = true
	ds.mu.Unlock()

	go ds.startPolling(ctx)
	ds.logger.InfoContext(ctx, "Started")

	return err
}

func (ds *PollDataSource) Close() error {
	ds.mu.RLock()
	ready := ds.ready
	ds.mu.RUnlock()

	if !ready {
		return fmt.Errorf("Datasource is not ready")
	}
	ds.logger.Info("Closing")
	ds.cancel()
	return nil
}

func (ds *PollDataSource) startPolling(ctx context.Context) {
	timer := time.NewTimer(ds.nextWait())
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			ds.mu.Lock()
			ds.ready = false
			ds.mu.Unlock()
			ds.logger.InfoContext(ctx, "Finished polling due to context")
			return
		case <-timer.C:
			err := ds.loadData(ctx)
			if err != nil {
				ds.logger.ErrorContext(ctx, "Error loading features", "error", err)
			}
			if errors.Is(err, context.Canceled) {
				ds.logger.InfoContext(ctx, "Finished polling due to context")
				return
			}

			ds.recordOutcome(ctx, err == nil)
			timer.Reset(ds.nextWait())
		}
	}
}

func (ds *PollDataSource) recordOutcome(ctx context.Context, succeeded bool) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if succeeded {
		ds.failures = 0
		return
	}

	ds.failures++
	wait := retryDelay(ds.interval, ds.failures)
	ds.logger.WarnContext(ctx, "Feature fetch failed",
		"attempt", ds.failures, "maxAttempts", maxRetryAttempts, "nextAttemptIn", wait)
}

func (ds *PollDataSource) nextWait() time.Duration {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return retryDelay(ds.interval, ds.failures)
}

func (ds *PollDataSource) loadData(ctx context.Context) error {
	ds.mu.RLock()
	etag := ds.etag
	ds.mu.RUnlock()

	resp, err := ds.client.CallFeatureApi(ctx, etag)
	if err != nil {
		return err
	}

	if resp.Etag != "" {
		ds.mu.Lock()
		ds.etag = resp.Etag
		ds.mu.Unlock()
	}

	if resp.Features == nil {
		return nil
	}

	err = ds.client.UpdateFromApiResponse(resp)
	if err != nil {
		return err
	}

	return nil
}
