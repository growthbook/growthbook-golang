package growthbook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/condition"
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

func TestExperimentStatus_StoppedForceStillRequiresEligibility(t *testing.T) {
	c := newAttrClient(t)
	force := 1
	exp := Experiment{
		Key:           "exp",
		Variations:    []FeatureValue{0, 1},
		Status:        StoppedStatus,
		Force:         &force,
		HashAttribute: "missing-id",
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
	require.Equal(t, 0, res.VariationId)

	exp.HashAttribute = ""
	exp.Condition = mustCondition(t, `{"country": "CA"}`)
	res = c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
	require.Equal(t, 0, res.VariationId)
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

func TestExperimentURLPatterns_CheckedBeforeForcedVariations(t *testing.T) {
	c := newAttrClient(t, WithForcedVariations(ForcedVariationsMap{"exp": 1}))
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
	require.Equal(t, 0, res.VariationId)
}

func TestExperimentURLPatterns_CheckedBeforeStickyBucket(t *testing.T) {
	service := NewInMemoryStickyBucketService()
	err := service.SaveAssignments(&StickyBucketAssignmentDoc{
		AttributeName:  "id",
		AttributeValue: "user-1",
		Assignments: map[string]string{
			getStickyBucketExperimentKey("exp", 0): "one",
		},
	})
	require.NoError(t, err)

	c := newAttrClient(t, WithStickyBucketService(service))
	c, err = c.WithUrl("https://example.com/home")
	require.NoError(t, err)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		Meta:       []VariationMeta{{Key: "zero"}, {Key: "one"}},
		URLPatterns: []URLTarget{
			{Type: URLTargetSimple, Pattern: "https://example.com/checkout"},
		},
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
	require.Equal(t, 0, res.VariationId)
}

func TestExperimentLegacyURL_MatchesFullURL(t *testing.T) {
	c := newAttrClient(t)
	c, err := c.WithUrl("https://example.com/checkout")
	require.NoError(t, err)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		URL:        `^https://example\.com/checkout$`,
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.True(t, res.InExperiment)
}

func TestExperimentLegacyURL_MatchesPathOnly(t *testing.T) {
	c := newAttrClient(t)
	c, err := c.WithUrl("https://example.com/checkout?step=1")
	require.NoError(t, err)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		URL:        `^/checkout\?step=1$`,
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.True(t, res.InExperiment)
}

func TestExperimentLegacyURL_NoMatchSkipped(t *testing.T) {
	c := newAttrClient(t)
	c, err := c.WithUrl("https://example.com/home")
	require.NoError(t, err)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		URL:        `^/checkout$`,
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
}

func TestExperimentLegacyURL_CheckedWithStickyBucket(t *testing.T) {
	service := NewInMemoryStickyBucketService()
	err := service.SaveAssignments(&StickyBucketAssignmentDoc{
		AttributeName:  "id",
		AttributeValue: "user-1",
		Assignments: map[string]string{
			getStickyBucketExperimentKey("exp", 0): "one",
		},
	})
	require.NoError(t, err)

	c := newAttrClient(t, WithStickyBucketService(service))
	c, err = c.WithUrl("https://example.com/home")
	require.NoError(t, err)
	exp := Experiment{
		Key:        "exp",
		Variations: []FeatureValue{0, 1},
		Meta:       []VariationMeta{{Key: "zero"}, {Key: "one"}},
		URL:        `^/checkout$`,
	}
	res := c.RunExperiment(context.Background(), &exp)
	require.False(t, res.InExperiment)
	require.Equal(t, 0, res.VariationId)
}

func mustCondition(t *testing.T, raw string) condition.Base {
	t.Helper()
	var cond condition.Base
	require.NoError(t, json.Unmarshal([]byte(raw), &cond))
	return cond
}
