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
	if err != nil {
		client.data.withLock(func(d *data) error {
			d.dsStartErr = err
			d.dsStarted = false
			return nil
		})
		return
	}

	client.data.withLock(func(d *data) error {
		d.dsStarted = true
		d.dsStartErr = nil
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
