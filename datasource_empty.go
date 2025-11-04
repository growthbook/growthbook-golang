package growthbook

import (
	"context"
)

type emptyDataSource struct {
	client *Client
}

var _ DataSource = &emptyDataSource{}

func withEmptyDataSource() ClientOption {
	return func(c *Client) error {
		c.data.dataSource = newEmptyDataSource(c)
		return nil
	}
}

func newEmptyDataSource(client *Client) *emptyDataSource {
	return &emptyDataSource{client}
}

func (ds *emptyDataSource) Start(ctx context.Context) error {
	ds.client.logger.InfoContext(ctx, "Starting empty data source")
	return nil
}

func (ds *emptyDataSource) Close() error {
	ds.client.logger.Info("Closing empty data source")
	return nil
}
