package growthbook

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/tmaxmax/go-sse"
)

type SseDataSource struct {
	client *Client
	cancel context.CancelFunc
	ready  bool
	// retry  time.Duration
	logger *slog.Logger
	mu     sync.RWMutex
}

const minbufsize = 64 * 1024
const maxbufsize = 10 * 1024 * 1024

func WithSseDataSource() ClientOption {
	return func(c *Client) error {
		c.data.dataSource = newSseDataSource(c)
		return nil
	}
}

func newSseDataSource(client *Client) *SseDataSource {
	return &SseDataSource{
		client: client,
		logger: client.logger.With("source", "Growthbook SSE datasource"),
	}
}

func (ds *SseDataSource) Start(ctx context.Context) error {
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
	go ds.connect(ctx)
	ds.logger.InfoContext(ctx, "Started")

	return nil
}

func (ds *SseDataSource) Close() error {
	ds.mu.RLock()
	ready := ds.ready
	ds.mu.RUnlock()

	if !ready {
		return fmt.Errorf("datasource is not ready")
	}
	ds.logger.Info("Closing")
	ds.cancel()
	return nil
}

func (ds *SseDataSource) connect(ctx context.Context) error {
	sseUrl := ds.client.data.getSseUrl()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sseUrl, http.NoBody)
	if err != nil {
		return err
	}

	ds.setReqHeaders(req)
	sseClient := &sse.Client{
		HTTPClient: ds.client.data.httpClient,
		OnRetry:    ds.onRetry(ctx),
	}
	sseConn := sseClient.NewConnection(req)
	buf := make([]byte, minbufsize)
	sseConn.Buffer(buf, maxbufsize)
	sseConn.SubscribeEvent("features", func(event sse.Event) {
		ds.processEvent(event)
	})
	sseConn.Connect()
	return nil
}

func (ds *SseDataSource) onRetry(ctx context.Context) func(err error, delay time.Duration) {
	return func(err error, delay time.Duration) {
		ds.logger.InfoContext(ctx, "Reconnect", "reason", err, "delay", delay)
		if err := ds.loadData(ctx); err != nil {
			ds.logger.ErrorContext(ctx, "Error loading features", "error", err)
		}
	}
}

func (ds *SseDataSource) processEvent(event sse.Event) {
	if event.Data == "" {
		return
	}
	ds.logger.Info("Updating features")
	err := ds.client.UpdateFromApiResponseJSON(event.Data)
	if err != nil {
		ds.logger.Error("Error updating features", "error", err)
	}
}

func (ds *SseDataSource) loadData(ctx context.Context) error {
	resp, err := ds.client.CallFeatureApi(ctx, "")
	if err != nil {
		return err
	}

	if !resp.SseSupport {
		return fmt.Errorf("sse is not supported")
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

func (ds *SseDataSource) setReqHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache", "no-cache")
}
