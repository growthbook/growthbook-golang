package growthbook

import "context"

// Plugin is an interface for extending the GrowthBook client with
// additional behavior such as tracking. Plugins are initialized when
// the client is created and closed when the client is closed.
//
// Contract for implementors: OnExperimentViewed, OnFeatureEvaluated,
// and Close MUST be safe to call even when Init returned an error.
// The client keeps failed plugins in its list and will still call
// these methods — guard your implementation accordingly.
type Plugin interface {
	// Init is called after the client is fully configured. The plugin
	// receives a reference to the client and may read configuration
	// from it (e.g., ClientKey). Returning an error marks the plugin
	// as failed; the client logs the error and continues. The client
	// will still call OnExperimentViewed, OnFeatureEvaluated, and
	// Close on a failed plugin.
	Init(client *Client) error

	// OnExperimentViewed is called when a user is included in an
	// experiment. Implementations should return quickly (e.g., by
	// enqueueing work). Panics are recovered by the caller.
	// Must be a no-op when Init returned an error.
	OnExperimentViewed(ctx context.Context, experiment *Experiment, result *ExperimentResult)

	// OnFeatureEvaluated is called every time a feature is evaluated.
	// Implementations should return quickly. Panics are recovered by
	// the caller.
	// Must be a no-op when Init returned an error.
	OnFeatureEvaluated(ctx context.Context, featureKey string, result *FeatureResult)

	// Close performs cleanup. For tracking plugins this means flushing
	// any remaining events. Close must be safe to call multiple times
	// and must be safe to call when Init returned an error.
	Close() error
}
