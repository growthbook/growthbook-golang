package growthbook

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubscribe_FiresOnInExperiment(t *testing.T) {
	c := newAttrClient(t)
	var calls atomic.Int32
	unsub := c.Subscribe(func(ctx context.Context, exp *Experiment, res *ExperimentResult) {
		calls.Add(1)
	})
	defer unsub()

	exp := Experiment{Key: "exp", Variations: []FeatureValue{0, 1}}
	c.RunExperiment(context.Background(), &exp)
	require.Equal(t, int32(1), calls.Load())
}

func TestSubscribe_FiresEvenWhenNotInExperiment(t *testing.T) {
	// Matches JS behavior: subscribers receive every run() result, including
	// misses. Callers can filter on result.InExperiment.
	c := newAttrClient(t)
	var calls atomic.Int32
	var lastInExperiment bool
	c.Subscribe(func(ctx context.Context, exp *Experiment, res *ExperimentResult) {
		calls.Add(1)
		lastInExperiment = res.InExperiment
	})

	exp := Experiment{Key: "exp", Variations: []FeatureValue{0, 1}, Status: DraftStatus}
	c.RunExperiment(context.Background(), &exp)
	require.Equal(t, int32(1), calls.Load())
	require.False(t, lastInExperiment)
}

func TestSubscribe_UnsubscribeStopsCalls(t *testing.T) {
	c := newAttrClient(t)
	var calls atomic.Int32
	unsub := c.Subscribe(func(ctx context.Context, exp *Experiment, res *ExperimentResult) {
		calls.Add(1)
	})

	exp := Experiment{Key: "exp", Variations: []FeatureValue{0, 1}}
	c.RunExperiment(context.Background(), &exp)
	unsub()
	c.RunExperiment(context.Background(), &exp)
	require.Equal(t, int32(1), calls.Load())
}

func TestSubscribe_SharedAcrossChildClients(t *testing.T) {
	c := newAttrClient(t)
	var calls atomic.Int32
	c.Subscribe(func(ctx context.Context, exp *Experiment, res *ExperimentResult) {
		calls.Add(1)
	})

	child, err := c.WithAttributes(Attributes{"id": "user-2"})
	require.NoError(t, err)

	exp := Experiment{Key: "exp", Variations: []FeatureValue{0, 1}}
	child.RunExperiment(context.Background(), &exp)
	require.Equal(t, int32(1), calls.Load())
}

func TestSubscribe_PanicIsRecovered(t *testing.T) {
	c := newAttrClient(t)
	c.Subscribe(func(ctx context.Context, exp *Experiment, res *ExperimentResult) {
		panic("boom")
	})
	exp := Experiment{Key: "exp", Variations: []FeatureValue{0, 1}}
	// Should not panic.
	c.RunExperiment(context.Background(), &exp)
}
