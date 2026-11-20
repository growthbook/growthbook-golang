package growthbook

import "context"

type DataSource interface {
	Start(context.Context) error
	Close() error
}

func WithDataSource(dataSource DataSource) ClientOption {
	return func(c *Client) error {
		c.data.dataSource = dataSource
		return nil
	}
}

func (client *Client) startDataSource(ctx context.Context) {
	defer close(client.data.dsStartWait)
	ds := client.data.dataSource

	err := ds.Start(ctx)

	client.data.withLock(func(d *data) error {
		d.dsStartErr = err
		// Marked started even when the first load failed: a data source that has begun polling or
		// connecting must still be reachable by Close, or its goroutine outlives the client.
		d.dsStarted = true
		return nil
	})
}

// EnsureLoaded blocks until the data source has finished its first load, and reports that load's
// outcome - not the client's present state. A source that failed at startup and recovered on a later
// poll still returns the original error here, by design: this answers "did startup succeed", which is
// what a caller gating on it at boot asks. Evaluate a feature to see what the client holds now.
func (client *Client) EnsureLoaded(ctx context.Context) error {
	select {
	case <-client.data.dsStartWait:
		return client.data.getDsStartErr()
	case <-ctx.Done():
		return ctx.Err()
	}
}
