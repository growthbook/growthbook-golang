package growthbook

import (
	"context"
	"runtime/debug"
	"sync"
)

// Plugin is an interface for extending the GrowthBook client with
// additional behavior such as tracking. Plugins are initialized when
// the client is created and closed when the client is closed.
type Plugin interface {
	// Init is called after the client is fully configured. The plugin
	// receives a reference to the client and may read configuration
	// from it (e.g., ClientKey).
	Init(client *Client) error

	// OnExperimentViewed is called when a user is included in an
	// experiment. Implementations should return quickly (e.g., by
	// enqueueing work). Panics are recovered by the caller.
	OnExperimentViewed(ctx context.Context, experiment *Experiment, result *ExperimentResult)

	// OnFeatureEvaluated is called every time a feature is evaluated.
	// Implementations should return quickly. Panics are recovered by
	// the caller.
	OnFeatureEvaluated(ctx context.Context, featureKey string, result *FeatureResult)

	// Close performs cleanup. For tracking plugins this means flushing
	// any remaining events. Close must be safe to call multiple times.
	Close() error
}

var (
	sdkVersionOnce  sync.Once
	sdkVersionValue string
)

// sdkVersion returns the module version of the growthbook-golang SDK,
// derived from go.mod via runtime/debug.ReadBuildInfo. The result is
// cached after the first call.
func sdkVersion() string {
	sdkVersionOnce.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			sdkVersionValue = "unknown"
			return
		}
		// When used as a dependency, the main module version is the
		// consumer's version. Walk the dependency list instead.
		for _, dep := range info.Deps {
			if dep.Path == "github.com/growthbook/growthbook-golang" {
				sdkVersionValue = dep.Version
				return
			}
		}
		// If this IS the main module (running tests, etc.), use the
		// main module version.
		if info.Main.Path == "github.com/growthbook/growthbook-golang" {
			sdkVersionValue = info.Main.Version
			if sdkVersionValue == "(devel)" || sdkVersionValue == "" {
				sdkVersionValue = "dev"
			}
			return
		}
		sdkVersionValue = "unknown"
	})
	return sdkVersionValue
}
