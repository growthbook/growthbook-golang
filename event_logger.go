package growthbook

import (
	"context"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// EventProperties holds the custom properties attached to a logged event.
type EventProperties map[string]any

// Built-in event names dispatched through the event-logger channel for
// every tracked exposure and feature evaluation, mirroring the JS SDK's
// EVENT_EXPERIMENT_VIEWED / EVENT_FEATURE_EVALUATED (sdk-js core.ts).
// Event logger callbacks can match on these to filter built-ins from
// custom LogEvent events.
const (
	EventExperimentViewed = "Experiment Viewed"
	EventFeatureEvaluated = "Feature Evaluated"
)

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
// receive events from the event-logger channel: custom events from
// [Client.LogEvent] and the built-in [EventExperimentViewed] /
// [EventFeatureEvaluated] events for every tracked exposure and feature
// evaluation. [GrowthBookTrackingPlugin] implements it, sending events to
// the GrowthBook ingestor through one batching pipeline.
type EventLoggerPlugin interface {
	OnEvent(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext)
}

// LogEvent logs a custom event, the Go equivalent of the JS SDK's
// logEvent and the Python SDK's log_event. The event is delivered to the
// event logger configured with [WithEventLogger] and to every plugin that
// implements [EventLoggerPlugin]. If neither is configured a warning is
// logged and the call is a no-op.
func (client *Client) LogEvent(ctx context.Context, eventName string, properties EventProperties) {
	if !client.dispatchEvent(ctx, eventName, properties, client.trackingUserContext()) {
		client.logger.WarnContext(ctx, "LogEvent called but no event logger is configured", "event", eventName)
	}
}

// hasEventConsumers reports whether anything listens on the event-logger
// channel: a WithEventLogger callback or a plugin implementing
// EventLoggerPlugin.
func (client *Client) hasEventConsumers() bool {
	if client.eventLogger != nil {
		return true
	}
	for _, p := range client.data.getPlugins() {
		if _, ok := p.(EventLoggerPlugin); ok {
			return true
		}
	}
	return false
}

// dispatchEvent fans one event out to the configured event logger and
// every EventLoggerPlugin. Returns true if any consumer received it.
func (client *Client) dispatchEvent(ctx context.Context, eventName string, properties EventProperties, userCtx *EventUserContext) bool {
	dispatched := false
	if client.eventLogger != nil {
		dispatched = true
		client.safeEventLogger(ctx, eventName, properties, userCtx)
	}
	for _, p := range client.data.getPlugins() {
		if ep, ok := p.(EventLoggerPlugin); ok {
			dispatched = true
			client.safePluginOnEvent(ctx, ep, eventName, properties, userCtx)
		}
	}
	return dispatched
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
