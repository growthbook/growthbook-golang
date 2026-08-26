package growthbook

import (
	"bytes"
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogEventCallback(t *testing.T) {
	ctx := context.TODO()
	var gotName string
	var gotProps EventProperties
	var gotUserCtx *EventUserContext

	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "123", "country": "US"}),
		WithUrl("http://example.com/checkout"),
		WithEventLogger(func(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext) {
			gotName = eventName
			gotProps = properties
			gotUserCtx = userCtx
		}),
		withSilentTestLogger(),
	)
	require.NoError(t, err)

	client.LogEvent(ctx, "button_clicked", EventProperties{"button": "buy"})

	require.Equal(t, "button_clicked", gotName)
	require.Equal(t, EventProperties{"button": "buy"}, gotProps)
	require.Equal(t, Attributes{"id": "123", "country": "US"}, gotUserCtx.Attributes)
	require.Equal(t, "http://example.com/checkout", gotUserCtx.URL)
}

func TestLogEventChildClientAttributes(t *testing.T) {
	ctx := context.TODO()
	var gotAttrs Attributes

	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "123"}),
		WithEventLogger(func(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext) {
			gotAttrs = userCtx.Attributes
		}),
		withSilentTestLogger(),
	)
	require.NoError(t, err)

	child, err := client.WithAttributes(Attributes{"id": "456"})
	require.NoError(t, err)

	child.LogEvent(ctx, "event", nil)
	require.Equal(t, Attributes{"id": "456"}, gotAttrs)
}

func TestLogEventNoLoggerWarns(t *testing.T) {
	ctx := context.TODO()
	var buf bytes.Buffer
	client, err := NewClient(ctx,
		WithLogger(slog.New(slog.NewTextHandler(&buf, nil))),
	)
	require.NoError(t, err)

	client.LogEvent(ctx, "orphan_event", nil)
	require.Contains(t, buf.String(), "no event logger is configured")
}

func TestLogEventLoggerPanicRecovered(t *testing.T) {
	ctx := context.TODO()
	var calls atomic.Int32
	client, err := NewClient(ctx,
		WithEventLogger(func(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext) {
			calls.Add(1)
			panic("boom")
		}),
		withSilentTestLogger(),
	)
	require.NoError(t, err)

	require.NotPanics(t, func() {
		client.LogEvent(ctx, "event", nil)
	})
	require.Equal(t, int32(1), calls.Load())
}
