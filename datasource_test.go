package growthbook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNilDataSource(t *testing.T) {
	client, err := NewClient(context.TODO())
	require.Nil(t, err)
	err = client.Close()
	require.Nil(t, err)
}

func TestEmptyDataSource(t *testing.T) {
	client, err := NewClient(context.TODO(), withEmptyDataSource())
	require.Nil(t, err)
	err = client.Close()
	require.Nil(t, err)
}
