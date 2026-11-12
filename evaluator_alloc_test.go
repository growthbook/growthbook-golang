package growthbook

import (
	"context"
	"testing"
)

// TestEvalFeatureAllocs pins the allocations of evaluating a feature with no
// rules: at most the returned FeatureResult. More means the evaluator or its
// prerequisite stack has moved to the heap.
func TestEvalFeatureAllocs(t *testing.T) {
	c, err := NewClient(context.Background(),
		WithAttributes(Attributes{"id": "user"}),
		WithFeatures(FeatureMap{"flag": {DefaultValue: true}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	allocs := testing.AllocsPerRun(100, func() {
		_ = c.EvalFeature(ctx, "flag")
	})
	if allocs > 1 {
		t.Errorf("EvalFeature allocated %v times per call, want at most 1", allocs)
	}
}
