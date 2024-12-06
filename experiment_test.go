package growthbook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExperimentWithNilAttributeFails(t *testing.T) {
	exp := Experiment{
		Key:        "my-test",
		Variations: []FeatureValue{0, 1},
	}

	c, _ := NewClient(
		context.TODO(),
		WithAttributes(Attributes{"id": nil}))

	res := c.RunExperiment(context.TODO(), &exp)
	require.False(t, res.InExperiment)
	require.False(t, res.HashUsed)
	require.Equal(t, 0, res.Value)
}

func TestExperimentWithMissingAttributeFails(t *testing.T) {
	exp := Experiment{
		Key:        "my-test",
		Variations: []FeatureValue{0, 1},
	}

	c, _ := NewClient(
		context.TODO(),
		WithAttributes(Attributes{}))

	res := c.RunExperiment(context.TODO(), &exp)
	require.False(t, res.InExperiment)
	require.False(t, res.HashUsed)
	require.Equal(t, 0, res.Value)
}
