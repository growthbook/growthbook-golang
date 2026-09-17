package growthbook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/tmaxmax/go-sse"
)

type SseDataSource struct {
	client           *Client
	cancel           context.CancelFunc
	ready            bool
	loaded           bool
	maxRetryInterval time.Duration
	logger           *slog.Logger
	mu               sync.RWMutex
}

// SseOption configures an SseDataSource.
type SseOption func(*SseDataSource)

// WithSseMaxRetryInterval sets the maximum backoff interval between reconnection
// attempts. Non-positive values keep the default cap.
func WithSseMaxRetryInterval(d time.Duration) SseOption {
	return func(ds *SseDataSource) {
		if d > 0 {
			ds.maxRetryInterval = d
		}
	}
}

const minbufsize = 64 * 1024
const maxbufsize = 10 * 1024 * 1024

// defaultMaxRetryInterval is the default cap for backoff delay between SSE reconnects.
const defaultMaxRetryInterval = 30 * time.Second

// errSseUnsupported reports a features response from a host that does not serve the stream. The
// features it carried have still been applied by the time this is returned.
var errSseUnsupported = errors.New("sse is not supported")

func WithSseDataSource(opts ...SseOption) ClientOption {
	return func(c *Client) error {
		ds := newSseDataSource(c)
		for _, opt := range opts {
			if opt != nil {
				opt(ds)
			}
		}
		c.data.dataSource = ds
		return nil
	}
}

func newSseDataSource(client *Client) *SseDataSource {
	return &SseDataSource{
		client:           client,
		maxRetryInterval: defaultMaxRetryInterval,
		logger:           client.logger.With("source", "Growthbook SSE datasource"),
	}
}

func (ds *SseDataSource) Start(ctx context.Context) error {
	ds.logger.InfoContext(ctx, "Starting")

	ctx, cancel := context.WithCancel(ctx)
	ds.cancel = cancel

	err := ds.loadData(ctx)

	if err == nil {
		ds.logger.InfoContext(ctx, "First load finished")
	}

	ds.mu.Lock()
	ds.ready = true
	ds.mu.Unlock()

	// The stream carries updates only, and a reload happens on reconnect alone, so a connection
	// that succeeds on the first try leaves the client empty until the next feature change -
	// hours, on a stable set of flags. Retry the initial load on its own. A load that delivered
	// features before failing (an SSE-less host) needs no retry.
	if err != nil && !ds.hasLoaded() {
		go ds.retryFirstLoad(ctx)
	}

	go ds.connect(ctx)
	ds.logger.InfoContext(ctx, "Started")

	return err
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

// retryFirstLoad reloads the features until one attempt lands, backing off between them. It stops as
// soon as any other path - a reconnect's reload, or a features event - has filled the client data,
// and gives up on a response the host will never answer differently.
func (ds *SseDataSource) retryFirstLoad(ctx context.Context) {
	failures := 1 // Start's own attempt already failed.

	timer := time.NewTimer(sseRetryDelay(ds.maxRetryInterval, failures))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if ds.hasLoaded() {
			return
		}

		err := ds.loadData(ctx)
		if err == nil {
			ds.logger.InfoContext(ctx, "First load finished")
			return
		}

		if errors.Is(err, context.Canceled) {
			return
		}

		var apiErr *FeatureApiError
		if errors.As(err, &apiErr) && !apiErr.Retryable() {
			ds.logger.ErrorContext(ctx, "Initial feature load cannot succeed, giving up", "error", err)
			return
		}

		failures++
		wait := sseRetryDelay(ds.maxRetryInterval, failures)
		ds.logger.WarnContext(ctx, "Initial feature load failed",
			"attempt", failures, "nextAttemptIn", wait, "error", err)
		timer.Reset(wait)
	}
}

// sseRetryDelay backs off 1s, 2s, 4s ... and settles at max. The poller's retryDelay is wrong here:
// it clips the delay to its steady polling interval, and an SSE source has no such interval to fall
// back on - maxRetryInterval is a ceiling, so the wait has to grow towards it rather than be capped
// into a fixed cadence by it.
func sseRetryDelay(max time.Duration, failures int) time.Duration {
	if max <= 0 {
		max = defaultMaxRetryInterval
	}

	// Beyond this the shift below would overflow, and the answer is max either way.
	if failures > 30 {
		return max
	}

	if failures < 1 {
		failures = 1
	}

	delay := baseRetryDelay << (failures - 1)
	if delay > max {
		return max
	}

	return delay
}

func (ds *SseDataSource) markLoaded() {
	ds.mu.Lock()
	ds.loaded = true
	ds.mu.Unlock()
}

func (ds *SseDataSource) hasLoaded() bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return ds.loaded
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
		Backoff: sse.Backoff{
			MaxInterval: ds.maxRetryInterval,
		},
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
		return
	}
	ds.markLoaded()
}

func (ds *SseDataSource) loadData(ctx context.Context) error {
	resp, err := ds.client.CallFeatureApi(ctx, "")
	if err != nil {
		return err
	}

	// A 200 payload always applies: UpdateFromApiResponse preserves omitted
	// sections, so partial responses update just what they carry (a
	// features-only guard here used to drop bandit- or saved-groups-only
	// updates entirely). This path never sends an ETag, so there is no 304
	// to skip.
	//
	// Applied before the stream is judged below: the features are valid whether or not the host
	// serves SSE, and discarding them over a missing header - a proxy that strips it, say - left
	// the client empty while holding a perfectly good payload.
	err = ds.client.UpdateFromApiResponse(resp)
	if err != nil {
		return err
	}
	ds.markLoaded()

	if !resp.SseSupport {
		return errSseUnsupported
	}

	return nil
}

func (ds *SseDataSource) setReqHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache", "no-cache")
}
