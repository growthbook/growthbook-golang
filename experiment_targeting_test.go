package growthbook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func newAttrClient(t *testing.T, opts ...ClientOption) *Client {
	t.Helper()
	base := []ClientOption{WithAttributes(Attributes{"id": "user-1"})}
	c, err := NewClient(context.Background(), append(base, opts...)...)
	require.NoError(t, err)
	return c
}

func TestExperimentStatus_StoppedHonorsForce(t *testing.T) {
	// Stopped + Force returns the forced variation. Matches Go's existing
	// forced-variation contract (InExperiment=true whenever the variation
	// index is valid); cleaning up "valid variation but not in experiment"
	// semantics is a separate change.
	c := newAttrClient(t)
	force := 1
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		Status:     StoppedStatus,
		Force:      &force,
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.Equal(t, 1, res.VariationId)
	require.False(t, res.HashUsed)
}

func TestExperimentStatus_StoppedNoForceReturnsControl(t *testing.T) {
	c := newAttrClient(t)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		Status:     StoppedStatus,
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
	require.Equal(t, 0, res.VariationId)
}

func TestExperimentStatus_DraftSkipped(t *testing.T) {
	c := newAttrClient(t)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		Status:     DraftStatus,
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
}

func TestExperimentGroups_NoMatch(t *testing.T) {
	c := newAttrClient(t, WithGroups(map[string]bool{"alpha": false, "beta": true}))
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		Groups:     []string{"alpha"},
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
}

func TestExperimentGroups_OneMatchIsEnough(t *testing.T) {
	c := newAttrClient(t, WithGroups(map[string]bool{"alpha": false, "beta": true}))
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		Groups:     []string{"alpha", "beta"},
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.True(t, res.InExperiment)
}

func TestExperimentGroups_NoUserGroupsRejects(t *testing.T) {
	c := newAttrClient(t)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		Groups:     []string{"alpha"},
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
}

func TestExperimentURLPatterns_Match(t *testing.T) {
	c := newAttrClient(t)
	c, err := c.WithUrl("https://example.com/checkout")
	require.NoError(t, err)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		URLPatterns: []URLTarget{
			{Type: URLTargetSimple, Pattern: "https://example.com/checkout"},
		},
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.True(t, res.InExperiment)
}

func TestExperimentURLPatterns_NoMatchSkipped(t *testing.T) {
	c := newAttrClient(t)
	c, err := c.WithUrl("https://example.com/home")
	require.NoError(t, err)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		URLPatterns: []URLTarget{
			{Type: URLTargetSimple, Pattern: "https://example.com/checkout"},
		},
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
}

func TestExperimentURLPatterns_NoClientURLNoMatch(t *testing.T) {
	c := newAttrClient(t)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		URLPatterns: []URLTarget{
			{Type: URLTargetSimple, Pattern: "https://example.com/checkout"},
		},
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
}
