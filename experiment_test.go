package growthbook

import (
	"context"
	"encoding/json"
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

func TestExperimentWithNoVariationsDoesNotPanic(t *testing.T) {
	exp := Experiment{
		Key:        "my-test",
		Variations: []FeatureValue{},
	}

	c, _ := NewClient(
		context.TODO(),
		WithAttributes(Attributes{"id": "user-1"}))

	res := c.RunExperiment(context.TODO(), &exp)
	require.False(t, res.InExperiment)
	require.False(t, res.HashUsed)
	require.Nil(t, res.Value)
}

func TestConditionlessExperimentJSONRoundTrip(t *testing.T) {
	// The deferred-tracking deep copy is a JSON round trip; an experiment
	// with no condition must survive it (zero conditions marshal as {}).
	var exp Experiment
	require.NoError(t, json.Unmarshal([]byte(`{"key": "e", "variations": ["a", "b"]}`), &exp))

	b, err := json.Marshal(exp)
	require.NoError(t, err)
	require.Contains(t, string(b), `"condition":{}`)

	var back Experiment
	require.NoError(t, json.Unmarshal(b, &back))
	require.Equal(t, exp.Key, back.Key)
}
