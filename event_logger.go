package growthbook

import (
	"context"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// EventProperties holds the custom properties attached to a logged event.
type EventProperties map[string]any

// EventUserContext carries the user context a custom event was logged
// with: the calling client's attributes and URL. It is the Go equivalent
// of the userContext argument the JS and Python SDKs pass to their event
// loggers.
type EventUserContext = TrackingUserContext

// EventLogger is invoked by [Client.LogEvent] for every custom event.
// Implementations should return quickly (e.g. by enqueueing work);
// panics are recovered by the caller.
type EventLogger func(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext)

// EventLoggerPlugin is an optional interface a [Plugin] can implement to
// receive custom events from [Client.LogEvent]. [GrowthBookTrackingPlugin]
// implements it, sending events to the GrowthBook ingestor through the
// same batching pipeline as experiment and feature events.
type EventLoggerPlugin interface {
	OnEvent(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext)
}

// LogEvent logs a custom event, the Go equivalent of the JS SDK's
// logEvent and the Python SDK's log_event. The event is delivered to the
// event logger configured with [WithEventLogger] and to every plugin that
// implements [EventLoggerPlugin]. If neither is configured a warning is
// logged and the call is a no-op.
func (client *Client) LogEvent(ctx context.Context, eventName string, properties EventProperties) {
	userCtx := client.trackingUserContext()

	logged := false
	if client.eventLogger != nil {
		logged = true
		client.safeEventLogger(ctx, eventName, properties, userCtx)
	}
	for _, p := range client.data.getPlugins() {
		if ep, ok := p.(EventLoggerPlugin); ok {
			logged = true
			client.safePluginOnEvent(ctx, ep, eventName, properties, userCtx)
		}
	}
	if !logged {
		client.logger.WarnContext(ctx, "LogEvent called but no event logger is configured", "event", eventName)
	}
}

// safeEventLogger calls the configured event logger, recovering panics.
func (client *Client) safeEventLogger(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext) {
	defer func() {
		if r := recover(); r != nil {
			client.logger.ErrorContext(ctx, "Event logger panicked", "error", r)
		}
	}()
	client.eventLogger(ctx, eventName, properties, userCtx)
}

// safePluginOnEvent calls the plugin's OnEvent, recovering panics.
func (client *Client) safePluginOnEvent(ctx context.Context, p EventLoggerPlugin, eventName string, properties EventProperties, userCtx *EventUserContext) {
	defer func() {
		if r := recover(); r != nil {
			client.logger.ErrorContext(ctx, "Plugin panicked in OnEvent", "error", r)
		}
	}()
	p.OnEvent(ctx, eventName, properties, userCtx)
}

// attributesFromValue converts the client's internal attribute
// representation back to plain Attributes.
func attributesFromValue(obj value.ObjValue) Attributes {
	attrs := make(Attributes, len(obj))
	for k, v := range obj {
		attrs[k] = value.ToAny(v)
	}
	return attrs
}
