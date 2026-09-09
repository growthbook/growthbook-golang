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

func (client *Client) EnsureLoaded(ctx context.Context) error {
	select {
	case <-client.data.dsStartWait:
		return client.data.getDsStartErr()
	case <-ctx.Done():
		return ctx.Err()
	}
}
