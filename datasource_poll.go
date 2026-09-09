package growthbook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type PollDataSource struct {
	client   *Client
	logger   *slog.Logger
	interval time.Duration
	cancel   context.CancelFunc
	ready    bool
	etag     string
	mu       sync.RWMutex
}

func WithPollDataSource(interval time.Duration) ClientOption {
	return func(c *Client) error {
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
	if err != nil {
		return err
	}
	ds.logger.InfoContext(ctx, "First load finished")

	ds.mu.Lock()
	ds.ready = true
	ds.mu.Unlock()
	go ds.startPolling(ctx)
	ds.logger.InfoContext(ctx, "Started")

	return nil
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
	ticker := time.NewTicker(ds.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ds.mu.Lock()
			ds.ready = false
			ds.mu.Unlock()
			ds.logger.InfoContext(ctx, "Finished polling due to context")
			return
		case <-ticker.C:
			err := ds.loadData(ctx)
			if err != nil {
				ds.logger.ErrorContext(ctx, "Error loading features", "error", err)
			}
			if errors.Is(err, context.Canceled) {
				ds.logger.InfoContext(ctx, "Finished polling due to context")
				return
			}
		}
	}
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

	// Skip only genuine no-update responses. A 200 payload always applies:
	// UpdateFromApiResponse preserves omitted sections, so a bandit-only or
	// saved-groups-only response updates just what it carries (a features-only
	// guard here used to drop those updates entirely).
	if resp.Status == http.StatusNotModified {
		return nil
	}

	err = ds.client.updateFromApiResponse(ctx, resp)
	if err != nil {
		return err
	}

	return nil
}
