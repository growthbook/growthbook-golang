package growthbook

import "context"

type DataSource interface {
	Start(context.Context) error
	Close() error
}

func (client *Client) startDataSource(ctx context.Context) {
	defer close(client.data.dsStartWait)
	ds := client.data.dataSource

	err := ds.Start(ctx)
	if err != nil {
		client.data.dsStartErr = err
		client.data.dsStarted = false
		return
	}

	client.data.dsStarted = true
	client.data.dsStartErr = nil
}

func (client *Client) EnsureLoaded(ctx context.Context) error {
	select {
	case <-client.data.dsStartWait:
		return client.data.dsStartErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
