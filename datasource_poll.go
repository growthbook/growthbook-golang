package growthbook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type PollDataSource struct {
	client   *Client
	logger   *slog.Logger
	interval time.Duration
	cancel   context.CancelFunc
	ready    bool
	etag     string
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
	ds.logger.Info("Starting")

	ctx, cancel := context.WithCancel(ctx)
	ds.cancel = cancel

	err := ds.loadData(ctx)
	if err != nil {
		return err
	}
	ds.logger.Info("First load finished")

	ds.ready = true
	go ds.startPolling(ctx)
	ds.logger.Info("Started")

	return nil
}

func (ds *PollDataSource) Close() error {
	if !ds.ready {
		return fmt.Errorf("Datasource is not ready")
	}
	ds.logger.Info("Closing")
	ds.cancel()
	return nil
}

func (ds *PollDataSource) startPolling(ctx context.Context) {
	timer := time.Tick(ds.interval)

	for {
		select {
		case <-ctx.Done():
			ds.ready = false
			ds.logger.Info("Finished polling due to context")
			return
		case <-timer:
			err := ds.loadData(ctx)
			if err != nil {
				ds.logger.Error("Error loading features", "error", err)
			}
			if errors.Is(err, context.Canceled) {
				ds.logger.Info("Finished polling due to context")
				return
			}
		}
	}
}

func (ds *PollDataSource) loadData(ctx context.Context) error {
	resp, err := ds.client.CallFeatureApi(ctx, ds.etag)
	if err != nil {
		return err
	}

	if resp.Etag != "" {
		ds.etag = resp.Etag
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
