package growthbook

import (
	"context"
	"fmt"
	"testing"
)

// buildBenchClient creates a client with the requested feature shape:
// numFeatures features, each with rulesPerFeature force rules guarded by a
// targeting condition. The first feature's last rule resolves to a force.
func buildBenchClient(b *testing.B, numFeatures, rulesPerFeature int) *Client {
	b.Helper()
	features := FeatureMap{}
	for i := 0; i < numFeatures; i++ {
		key := fmt.Sprintf("feature-%d", i)
		f := &Feature{DefaultValue: false}
		for j := 0; j < rulesPerFeature; j++ {
			force := FeatureValue(true)
			f.Rules = append(f.Rules, FeatureRule{
				Id:    fmt.Sprintf("%s-rule-%d", key, j),
				Force: force,
			})
		}
		features[key] = f
	}
	c, err := NewClient(context.Background(),
		WithAttributes(Attributes{"id": "bench-user", "country": "US", "plan": "pro"}),
		WithFeatures(features),
	)
	if err != nil {
		b.Fatal(err)
	}
	return c
}

func BenchmarkEvalFeature_Cold(b *testing.B) {
	c := buildBenchClient(b, 1, 1)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.EvalFeature(ctx, "feature-0")
	}
}

func BenchmarkEvalFeature_Warm(b *testing.B) {
	c := buildBenchClient(b, 50, 5)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.EvalFeature(ctx, "feature-25")
	}
}

func objectValuedBenchClient(b *testing.B, opt ClientOption) *Client {
	b.Helper()
	payload := map[string]any{}
	for i := 0; i < 40; i++ {
		payload[fmt.Sprintf("key-%d", i)] = map[string]any{"enabled": i%2 == 0, "weight": float64(i), "label": "value"}
	}
	c, err := NewClient(context.Background(),
		WithAttributes(Attributes{"id": "bench-user"}),
		WithFeatures(FeatureMap{"config": {DefaultValue: payload}}),
		opt,
	)
	if err != nil {
		b.Fatal(err)
	}
	return c
}

// An object-valued feature on a client whose only tracking consumer is an
// ExperimentCallback.
func BenchmarkEvalFeature_ObjectValue_ExperimentCallback(b *testing.B) {
	c := objectValuedBenchClient(b, WithExperimentCallback(func(context.Context, *Experiment, *ExperimentResult, *TrackingUserContext, any) {}))
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.EvalFeature(ctx, "config")
	}
}

// The same feature on a client with a FeatureUsageCallback.
func BenchmarkEvalFeature_ObjectValue_FeatureUsageCallback(b *testing.B) {
	c := objectValuedBenchClient(b, WithFeatureUsageCallback(func(context.Context, string, *FeatureResult, any) {}))
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.EvalFeature(ctx, "config")
	}
}

func BenchmarkRunExperiment(b *testing.B) {
	c, err := NewClient(context.Background(),
		WithAttributes(Attributes{"id": "bench-user"}),
	)
	if err != nil {
		b.Fatal(err)
	}
	exp := &Experiment{
		Key:        "bench-exp",
		Variations: []FeatureValue{0, 1, 2, 3},
		Weights:    []float64{0.25, 0.25, 0.25, 0.25},
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.RunExperiment(ctx, exp)
	}
}

func BenchmarkEvalFeature_Parallel(b *testing.B) {
	c := buildBenchClient(b, 50, 5)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = c.EvalFeature(ctx, "feature-25")
		}
	})
}

func BenchmarkIsURLTargeted_Simple(b *testing.B) {
	targets := []URLTarget{
		{Type: URLTargetSimple, Pattern: "https://*.example.com/checkout"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isURLTargeted("https://app.example.com/checkout", targets)
	}
}

func BenchmarkIsURLTargeted_Regex(b *testing.B) {
	targets := []URLTarget{
		{Type: URLTargetRegex, Pattern: `^https://example\.com/foo/\d+$`},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isURLTargeted("https://example.com/foo/42", targets)
	}
}
