package growthbook

import (
	"context"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestWithAttributeOverrides(t *testing.T) {
	ctx := context.TODO()

	t.Run("with initial attributes", func(t *testing.T) {
		client, err := NewClient(ctx, WithAttributes(Attributes{"user": 1}))
		require.NoError(t, err)

		child, err := client.WithAttributeOverrides(Attributes{"user": 2})
		require.NoError(t, err)

		// Original client should be unchanged
		require.Equal(t, value.ObjValue{"user": value.Num(1)}, client.attributes)
		// Child client should have new value
		require.Equal(t, value.ObjValue{"user": value.Num(2)}, child.attributes)
	})

	t.Run("without initial attributes", func(t *testing.T) {
		client, err := NewClient(ctx)
		require.NoError(t, err)

		child, err := client.WithAttributeOverrides(Attributes{"user": 2})
		require.NoError(t, err)

		// Child client should have the new attributes
		require.Equal(t, value.ObjValue{"user": value.Num(2)}, child.attributes)
	})

	t.Run("ignore nil attribute overrides", func(t *testing.T) {
		client, err := NewClient(ctx, WithAttributes(Attributes{"user": 1}))
		require.NoError(t, err)

		child, err := client.WithAttributeOverrides(nil)
		require.NoError(t, err)

		// Original client should be unchanged
		require.Equal(t, value.ObjValue{"user": value.Num(1)}, client.attributes)
		// Child client should have same attributes as parent
		require.Equal(t, value.ObjValue{"user": value.Num(1)}, child.attributes)
	})
}

func TestWithAttributes(t *testing.T) {
	ctx := context.TODO()

	t.Run("ignore nil initial attributes", func(t *testing.T) {
		client, err := NewClient(ctx, WithAttributes(nil))
		require.NoError(t, err)
		require.Equal(t, value.ObjValue{}, client.attributes)
	})

	t.Run("don't panic on nil attributes", func(t *testing.T) {
		client, err := NewClient(ctx, WithAttributes(Attributes{"user": 1}))
		require.NoError(t, err)
		child, err := client.WithAttributes(nil)
		require.NoError(t, err)
		// Original client should be unchanged
		require.Equal(t, value.ObjValue{"user": value.Num(1)}, client.attributes)
		// Child client should have empty attributes
		require.Equal(t, value.ObjValue{}, child.attributes)
	})
}
