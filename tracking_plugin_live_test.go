//go:build live_tracking_test

// Run with: go test -tags live_tracking_test -run TestLiveTrackingPlugin -v
//
// This test sends real events to the GrowthBook ingestor. Use it to
// verify end-to-end that the tracking plugin can reach the API.
//
// Set these environment variables before running:
//   GB_CLIENT_KEY   — your GrowthBook SDK client key (required)
//   GB_INGESTOR_HOST — ingestor endpoint (optional, defaults to https://us1.gb-ingest.com)
//
// After running, check the GrowthBook dashboard to confirm events arrived.
// This test file can be deleted once verification is complete.

package growthbook

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLiveTrackingPlugin(t *testing.T) {
	clientKey := os.Getenv("GB_CLIENT_KEY")
	if clientKey == "" {
		t.Skip("GB_CLIENT_KEY not set — skipping live tracking test")
	}

	ingestorHost := os.Getenv("GB_INGESTOR_HOST")
	if ingestorHost == "" {
		ingestorHost = defaultIngestorHost
	}

	t.Logf("Using client key: %s", clientKey)
	t.Logf("Ingestor host:    %s", ingestorHost)

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithClientKey(clientKey),
		WithAttributes(Attributes{
			"id":      "go-sdk-live-test-user",
			"country": "US",
		}),
		WithGrowthBookTracking(TrackingPluginConfig{
			IngestorHost: ingestorHost,
			BatchSize:    1, // flush every event immediately
			BatchTimeout: 1 * time.Second,
		}),
	)
	require.NoError(t, err)

	// Evaluate a feature — sends a feature_evaluated event.
	featuresJSON := `{
		"live-test-feature": {
			"defaultValue": true
		},
		"live-test-experiment": {
			"defaultValue": "control",
			"rules": [{
				"variations": ["control", "variant-a", "variant-b"],
				"weights": [0.34, 0.33, 0.33]
			}]
		}
	}`
	require.NoError(t, client.SetJSONFeatures(featuresJSON))

	res := client.EvalFeature(ctx, "live-test-feature")
	t.Logf("Feature 'live-test-feature' evaluated: value=%v, source=%s", res.Value, res.Source)

	res2 := client.EvalFeature(ctx, "live-test-experiment")
	t.Logf("Feature 'live-test-experiment' evaluated: value=%v, source=%s, inExperiment=%v",
		res2.Value, res2.Source, res2.InExperiment())

	// Run a standalone experiment.
	exp := &Experiment{
		Key:        "live-test-standalone-exp",
		Variations: []FeatureValue{"control", "treatment"},
		Name:       "Live Test Standalone Experiment",
	}
	expResult := client.RunExperiment(ctx, exp)
	t.Logf("Experiment 'live-test-standalone-exp': variation=%d, inExperiment=%v",
		expResult.VariationId, expResult.InExperiment)

	// Close flushes all remaining events.
	err = client.Close()
	require.NoError(t, err)

	t.Log("All events flushed. Check the GrowthBook dashboard for:")
	t.Log("  - feature_evaluated: live-test-feature")
	t.Log("  - feature_evaluated + experiment_viewed: live-test-experiment")
	t.Log("  - experiment_viewed: live-test-standalone-exp")
}
