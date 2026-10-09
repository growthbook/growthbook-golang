package growthbook

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type closeTestDataSource struct {
	calls atomic.Int32
	err   error
}

func (d *closeTestDataSource) Start(context.Context) error { return nil }
func (d *closeTestDataSource) Close() error                { d.calls.Add(1); return d.err }

type closeTestPlugin struct {
	calls atomic.Int32
	err   error
}

func (p *closeTestPlugin) Init(*Client) error                                                 { return nil }
func (p *closeTestPlugin) OnExperimentViewed(context.Context, *Experiment, *ExperimentResult) {}
func (p *closeTestPlugin) OnFeatureEvaluated(context.Context, string, *FeatureResult)         {}
func (p *closeTestPlugin) Close() error                                                       { p.calls.Add(1); return p.err }

func TestDerivedClientClosePreservesSharedResources(t *testing.T) {
	for _, failing := range []bool{false, true} {
		name := "successful cleanup"
		if failing {
			name = "cleanup errors"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			ds := &closeTestDataSource{}
			plugin := &closeTestPlugin{}
			if failing {
				ds.err = errors.New("data source close")
				plugin.err = errors.New("plugin close")
			}
			root, err := NewClient(ctx, WithDataSource(ds), WithPlugins(plugin))
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.EnsureLoaded(ctx))
			child, err := root.WithAttributes(Attributes{"id": "child"})
			require.NoError(t, err)
			grandchild, err := child.WithUrl("https://example.test")
			require.NoError(t, err)
			sibling, err := root.WithEnabled(true)
			require.NoError(t, err)
			for _, derived := range []*Client{child, grandchild, sibling} {
				require.NoError(t, derived.Close())
				require.NoError(t, derived.Close())
			}
			require.Zero(t, ds.calls.Load(), "derived clients must not close the shared data source")
			require.Zero(t, plugin.calls.Load(), "derived clients must not close shared plugins")
			err = root.Close()
			if failing {
				require.ErrorIs(t, err, ds.err)
				require.ErrorIs(t, err, plugin.err)
			} else {
				require.NoError(t, err)
			}
			require.EqualValues(t, 1, ds.calls.Load())
			require.EqualValues(t, 1, plugin.calls.Load())
		})
	}
}

func TestDerivedClientCloseKeepsTrackingAlive(t *testing.T) {
	srv, requests, mu := newTestIngestor(t)
	defer srv.Close()
	ctx := context.Background()
	root, err := NewClient(ctx, WithClientKey("sdk-test-key"), WithGrowthBookTracking(TrackingPluginConfig{
		IngestorHost: srv.URL, BatchSize: 100, BatchTimeout: time.Hour,
	}))
	require.NoError(t, err)
	defer root.Close()
	child, err := root.WithAttributes(Attributes{"id": "child"})
	require.NoError(t, err)
	sibling, err := root.WithAttributes(Attributes{"id": "sibling"})
	require.NoError(t, err)
	child.LogEvent(ctx, "before_child_close", nil)
	require.NoError(t, child.Close())
	require.Empty(t, getRequests(requests, mu), "child close must not flush the shared plugin")
	root.LogEvent(ctx, "root_after_child_close", nil)
	sibling.LogEvent(ctx, "sibling_after_child_close", nil)
	require.NoError(t, root.Close())
	events := eventsByName(t, getRequests(requests, mu), "sdk-test-key")
	for _, name := range []string{"before_child_close", "root_after_child_close", "sibling_after_child_close"} {
		require.Len(t, events[name], 1)
	}
}
