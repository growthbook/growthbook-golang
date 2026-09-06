![GrowthBook Go SDK Hero Image](growthbook-hero-go-sdks.png)

<div align="center">
<h1>GrowthBook Go SDK</h1>

[![Go Report Card](https://goreportcard.com/badge/github.com/growthbook/growthbook-golang)](https://goreportcard.com/report/github.com/growthbook/growthbook-golang)
[![GoDoc](https://pkg.go.dev/badge/github.com/growthbook/growthbook-golang)](https://pkg.go.dev/github.com/growthbook/growthbook-golang)
[![License](https://img.shields.io/github/license/growthbook/growthbook-golang)](https://github.com/growthbook/growthbook-golang/blob/main/LICENSE)
[![Release](https://img.shields.io/github/v/release/growthbook/growthbook-golang)](https://github.com/growthbook/growthbook-golang/releases/latest)

GrowthBook is a modular feature flagging and experimentation platform. You can use GrowthBook for feature flags, running no-code experiments with a visual editor, analyzing experiment results, or any combination of the above.
</div>

## Requirements

- Go version 1.21 or higher (tested with 1.21, 1.22, and 1.23)

---

## Installation

```bash
go get github.com/growthbook/growthbook-golang
```

---

## Usage

### Quick Start

```go
import (
    "context"
    "log"
    gb "github.com/growthbook/growthbook-golang"
)

// Create a new client instance with a client key and 
// a data source that loads features in the background via SSE stream.
// Pass the client's options to the NewClient function.
client, err := gb.NewClient(
    context.Background(),
    gb.WithClientKey("sdk-XXXX"),
    gb.WithSseDataSource(),
) 
defer client.Close()

if err != nil {
    log.Fatal("Client initialization failed: ", err)
}

// The data source starts asynchronously. Use EnsureLoaded to 
// wait until the client data is initialized for the first time.
if err := client.EnsureLoaded(context.Background()); err != nil {
    log.Fatal("Data loading failed: ", err)
}

// Create a child client with specific attributes.
attrs := gb.Attributes{"id": 100, "user": "user1"}
child, err := client.WithAttributes(attrs)
if err != nil {
    log.Fatal("Child client creation failed: ", err)
}

// Evaluate a text feature
buttonColor := child.EvalFeature(context.Background(), "buy-button-color")
if buttonColor.Value == "blue" {
    // Perform actions for blue button
}

// Evaluate a boolean feature
darkMode := child.EvalFeature(context.Background(), "dark-mode")
if darkMode.On {
    // Enable dark mode
}
```

---

### Client

The client is the core component of the GrowthBook SDK. After installing and importing the SDK, create a single shared instance of `growthbook.Client` using the `growthbook.NewClient` function with a list of options. You can customize the client with options like a custom logger, client key, decryption key, default attributes, or a feature list from JSON. The client is thread-safe and can be safely used from multiple goroutines.

While you can evaluate features directly using the main client instance, it's recommended to create child client instances that include session- or query-specific data. To create a child client with local attributes, call `client.WithAttributes`:

```go
attrs := gb.Attributes{"id": 100, "user": "Bob"}
child, err := client.WithAttributes(attrs)
```

Now, you can evaluate features using the child client:

```go
res := child.EvalFeature(context.Background(), "main-button-color")
```

Additional options, such as `WithLogger`, `WithUrl`, and `WithAttributesOverrides`, can also be used to customize child clients. Since child clients share data with the main client instance, they will automatically receive feature updates.

To stop background updates, call `client.Close()` on the main client instance when it is no longer needed.

---

### Tracking

#### Built-in GrowthBook Tracking Plugin

The SDK includes a built-in tracking plugin that automatically sends experiment and feature evaluation events to the GrowthBook warehouse. This is the easiest way to get analytics data flowing without any custom integration.

```go
client, err := gb.NewClient(
    context.Background(),
    gb.WithClientKey("sdk-XXXX"),
    gb.WithSseDataSource(),
    gb.WithGrowthBookTracking(gb.TrackingPluginConfig{
        // Defaults to "https://us1.gb-ingest.com"
        IngestorHost: "https://us1.gb-ingest.com",
    }),
)
defer client.Close() // flushes remaining events
```

The plugin batches events and sends them in the background. Configuration options:

| Option | Default | Description |
|--------|---------|-------------|
| `IngestorHost` | `https://us1.gb-ingest.com` | GrowthBook event ingestor endpoint |
| `BatchSize` | `100` | Max events before auto-flush |
| `BatchTimeout` | `10s` | Max wait time before auto-flush |
| `HTTPClient` | Client's HTTP client | Custom HTTP client for sending events |
| `Logger` | Client's logger | Custom logger for error reporting |

Events tracked automatically (standard GrowthBook event names, shared with
the JS SDK):
- **`Experiment Viewed`** — when a user is bucketed into an experiment
- **`Feature Evaluated`** — every time a feature flag is evaluated

Events are sent in the ingestor's standard `EventPayload` shape (`POST
{IngestorHost}/track?client_key=...` with a JSON array), carrying the
evaluating client's attributes — so exposures from child clients are
attributed to the right user.

If plugin initialization fails (e.g., missing client key), the plugin silently becomes a no-op — it never interferes with SDK evaluation.

#### Custom Events

`client.LogEvent` logs a custom event with arbitrary properties — the Go
equivalent of `logEvent` in the JS SDK and `log_event` in the Python SDK.
With the tracking plugin configured, custom events are batched and sent to
the ingestor alongside the automatic events:

```go
client.LogEvent(ctx, "button_clicked", gb.EventProperties{"button": "buy"})
```

You can also handle events yourself with `WithEventLogger` (with or
without the tracking plugin — like callbacks, both fire independently).
Matching the JS SDK, the event logger receives the built-in
`Experiment Viewed` / `Feature Evaluated` events as well as custom
`LogEvent` events — match on the `gb.EventExperimentViewed` /
`gb.EventFeatureEvaluated` constants to filter:

```go
client, err := gb.NewClient(
    context.Background(),
    gb.WithEventLogger(func(ctx context.Context, eventName string, properties gb.EventProperties, userCtx *gb.EventUserContext) {
        if eventName == gb.EventExperimentViewed || eventName == gb.EventFeatureEvaluated {
            return // only interested in custom events
        }
        // Send to your analytics provider; userCtx carries the evaluating
        // client's attributes and URL.
    }),
)
```

Custom plugins can receive these events too by implementing the optional
`EventLoggerPlugin` interface (`OnEvent`).

#### Custom Tracking via Callbacks

For custom analytics integrations, you can set up two callbacks:

1. **`ExperimentCallback`**: Triggered when a user is included in an
   experiment, with the user context (attributes and URL) the evaluation ran
   with — the equivalent of the JS SDK's `trackingCallback`.
2. **`FeatureUsageCallback`**: Triggered on each feature evaluation.

You can also attach extra data that will be sent with each callback. These callbacks can be set globally via the `NewClient` function using the `WithExperimentCallback` and `WithFeatureUsageCallback` options. Alternatively, you can set them locally when creating child clients using similar methods like `client.WithExperimentCallback`. Extra data is set via the `WithExtraData` option.

```go
client, err := gb.NewClient(
    context.Background(),
    gb.WithClientKey("sdk-XXXX"),
    gb.WithExperimentCallback(func(ctx context.Context, exp *gb.Experiment, result *gb.ExperimentResult, userCtx *gb.TrackingUserContext, extraData any) {
        // Send to your analytics provider
    }),
    gb.WithFeatureUsageCallback(func(ctx context.Context, key string, result *gb.FeatureResult, extraData any) {
        // Track feature usage
    }),
    gb.WithExtraData(myAnalyticsService),
)
```

Callbacks and the tracking plugin can be used together — they operate independently.

#### Deferred Tracking

Sometimes the process evaluating features isn't the right place to send
analytics from. A common example is a remote-evaluation server: it evaluates
features on behalf of client SDKs, and each client should report its own
exposures. Deferred tracking buffers every experiment exposure an evaluation
produces — including passthrough assignments and experiments inside
prerequisite features — so you can read the buffer and forward it, instead of
intercepting callbacks.

Enable it with `WithDeferredTracking`. The usual pattern is one child client
per user request; the child acts as the user context, so every request gets
its own buffer:

```go
child, _ := client.WithAttributes(gb.Attributes{"id": userID})
child, _ = child.WithDeferredTracking()

child.EvalFeature(ctx, "feature-a")
child.EvalFeature(ctx, "feature-b")

exposures := child.DeferredTrackingCalls() // []gb.TrackingData
```

Good to know:

- The buffer keeps one entry per unique assignment (same user, experiment,
  and variation), in the order they were first seen. Reads return detached
  copies, safe to retain or mutate; `ClearDeferredTrackingCalls` empties the
  buffer.
- `TrackingData` marshals to the same JSON shape as the JS SDK's tracking
  data — including the `user` context the evaluation ran with — so a
  forwarded list can be passed directly to a JS client's
  `setDeferredTrackingCalls`.
- Child clients cloned from an armed client share its buffer. Calling
  `WithDeferredTracking` again gives the new client a fresh, separate buffer.
- Arming a shared client also works — the buffer is safe for concurrent use
  and entries carry their user identity — but you lose the per-request
  boundary, so prefer arming per-request children.
- Callbacks and plugins are unaffected and keep firing. If you both forward
  the buffer and track via callbacks, you'll report exposures twice — pick
  one channel.
- Feature usage is not buffered, only experiment exposures. Usage events
  describe where evaluation happened, and remote-evaluation clients report
  their own; on the server they still reach callbacks and plugins.

#### Custom Plugins

You can implement the `Plugin` interface to create custom tracking or other plugins:

```go
type Plugin interface {
    Init(client *gb.Client) error
    OnExperimentViewed(ctx context.Context, experiment *gb.Experiment, result *gb.ExperimentResult)
    OnFeatureEvaluated(ctx context.Context, featureKey string, result *gb.FeatureResult)
    Close() error
}
```

Register custom plugins using `WithPlugins`:

```go
client, err := gb.NewClient(
    context.Background(),
    gb.WithClientKey("sdk-XXXX"),
    gb.WithPlugins(myCustomPlugin),
)
```

Plugins are shared with child clients. Any panics in plugin methods are recovered and logged — they never interrupt SDK evaluation.

---

### Sticky Bucketing

Sticky Bucketing ensures users see consistent experiment variations across sessions and devices. The SDK provides an in-memory implementation by default, but you can implement your own storage solution.

#### Basic Usage

```go
// Create an in-memory sticky bucket service
service := gb.NewInMemoryStickyBucketService()

// Create a client with sticky bucketing
client, err := gb.NewClient(
    context.Background(),
    gb.WithClientKey("sdk-XXXX"),
    gb.WithStickyBucketService(service),
)

// Run an experiment with sticky bucketing
exp := &gb.Experiment{
    Key:        "my-experiment",
    Variations: []gb.FeatureValue{"control", "treatment"},
    Meta: []gb.VariationMeta{
        {Key: "0"}, // Use numeric keys to match variation IDs
        {Key: "1"},
    },
    BucketVersion:    1,
    MinBucketVersion: 0,
}

result := client.RunExperiment(context.Background(), exp)
```

#### Custom Implementation

Implement the `StickyBucketService` interface for custom storage:

#### Concurrency & Caching

- The in-memory service implementation is thread-safe using `sync.RWMutex`
- Assignments are cached per client to reduce storage calls; the cache is
  shared between a client and its child clients (created via `With*` methods)
  and is bounded (LRU, 10,000 docs by default — tune or remove the bound with
  `WithStickyBucketCacheSize`)
- Within a client and its children, concurrent reads and saves of the same
  user's assignment doc are serialized, so parallel evaluations merge
  assignments instead of overwriting each other
- **Scope of the guarantee:** independently constructed clients — or separate
  processes — sharing one `StickyBucketService` are not serialized against
  each other. `SaveAssignments` writes whole documents last-write-wins, so
  truly concurrent saves for the same user from different clients can drop
  an assignment. If that matters for your deployment, make your service's
  `SaveAssignments` merge atomically in the backend (e.g. a Redis hash or a
  transactional upsert)

For more details, see the [official documentation](https://docs.growthbook.io/app/sticky-bucketing).

---

## Documentation

- [Usage Guide](https://docs.growthbook.io/lib/go)
- [GoDoc](https://pkg.go.dev/github.com/growthbook/growthbook-golang)

---
