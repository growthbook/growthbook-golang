package growthbook

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The rollout inclusion check hashes on the rule's fallback attribute when the primary hash attribute
// has no value, but only while sticky bucketing is active - matching the reference SDK, which passes
// `saveStickyBucketAssignmentDoc && !rule.disableStickyBucketing ? rule.fallbackAttribute : undefined`
// into isIncludedInRollout.
func TestRolloutUsesFallbackAttributeWhenStickyBucketingIsActive(t *testing.T) {
	featuresJson := `{
    "feature": {
      "defaultValue": 0,
      "rules": [{"force": 1, "coverage": 1.0, "hashAttribute": "id", "fallbackAttribute": "deviceId"}]
    }
  }`

	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithAttributes(Attributes{"deviceId": "device-1"}),
		WithStickyBucketService(NewInMemoryStickyBucketService()),
	)
	require.NoError(t, err)

	result := client.EvalFeature(ctx, "feature")

	require.Equal(t, ForceResultSource, result.Source,
		"the rule has no value for id but does for deviceId, so the fallback has to supply the hash value")
	require.EqualValues(t, 1, result.Value)
}

func TestRolloutIgnoresFallbackAttributeWithoutStickyBucketing(t *testing.T) {
	featuresJson := `{
    "feature": {
      "defaultValue": 0,
      "rules": [{"force": 1, "coverage": 1.0, "hashAttribute": "id", "fallbackAttribute": "deviceId"}]
    }
  }`

	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithAttributes(Attributes{"deviceId": "device-1"}),
	)
	require.NoError(t, err)

	result := client.EvalFeature(ctx, "feature")

	require.Equal(t, DefaultValueResultSource, result.Source,
		"without a sticky bucket service the fallback must not change which attribute a rollout hashes on")
}

func TestRolloutIgnoresFallbackAttributeWhenTheRuleDisablesStickyBucketing(t *testing.T) {
	featuresJson := `{
    "feature": {
      "defaultValue": 0,
      "rules": [{
        "force": 1, "coverage": 1.0,
        "hashAttribute": "id", "fallbackAttribute": "deviceId",
        "disableStickyBucketing": true
      }]
    }
  }`

	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithAttributes(Attributes{"deviceId": "device-1"}),
		WithStickyBucketService(NewInMemoryStickyBucketService()),
	)
	require.NoError(t, err)

	result := client.EvalFeature(ctx, "feature")

	require.Equal(t, DefaultValueResultSource, result.Source,
		"a rule opting out of sticky bucketing also opts out of the fallback attribute")
}

func TestRolloutPrefersThePrimaryHashAttributeOverTheFallback(t *testing.T) {
	featuresJson := `{
    "feature": {
      "defaultValue": 0,
      "rules": [{"force": 1, "coverage": 1.0, "hashAttribute": "id", "fallbackAttribute": "deviceId"}]
    }
  }`

	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithAttributes(Attributes{"id": "user-1", "deviceId": "device-1"}),
		WithStickyBucketService(NewInMemoryStickyBucketService()),
	)
	require.NoError(t, err)

	result := client.EvalFeature(ctx, "feature")

	require.Equal(t, ForceResultSource, result.Source)
}

// The two cases below come verbatim from the upstream spec suite at specVersion 0.8.0. The vendored
// cases.json is at 0.7.0 and carries neither, so nothing else covers a force rule that combines
// coverage with an explicit hashVersion.
func TestForceRuleHashVersion2IncludesUser(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "coverage": 0.5, "hashVersion": 2}]}
  }`

	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithAttributes(Attributes{"id": "user2"}),
	)
	require.NoError(t, err)

	result := client.EvalFeature(ctx, "feature")

	require.EqualValues(t, 1, result.Value)
	require.Equal(t, ForceResultSource, result.Source)
}

func TestForceRuleHashVersion2ExcludesUserThatVersion1WouldInclude(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "coverage": 0.5, "hashVersion": 2}]}
  }`

	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithAttributes(Attributes{"id": "user3"}),
	)
	require.NoError(t, err)

	result := client.EvalFeature(ctx, "feature")

	require.EqualValues(t, 0, result.Value)
	require.Equal(t, DefaultValueResultSource, result.Source)
}

// The versions have to disagree for the two cases above to mean anything: a hardcoded version would
// give both users the same answer regardless of what the rule declares.
func TestForceRuleHashVersionsBucketTheSameUsersOppositely(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		hashVersion int
		wantSource  FeatureResultSource
	}{
		{"user2 under v2", "user2", 2, ForceResultSource},
		{"user2 under v1", "user2", 1, DefaultValueResultSource},
		{"user3 under v2", "user3", 2, DefaultValueResultSource},
		{"user3 under v1", "user3", 1, ForceResultSource},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			featuresJson := `{
        "feature": {"defaultValue": 0, "rules": [{"force": 1, "coverage": 0.5, "hashVersion": ` +
				string(rune('0'+test.hashVersion)) + `}]}
      }`

			client, err := NewClient(ctx,
				WithJsonFeatures(featuresJson),
				WithAttributes(Attributes{"id": test.id}),
			)
			require.NoError(t, err)

			require.Equal(t, test.wantSource, client.EvalFeature(ctx, "feature").Source)
		})
	}
}

func TestForceRuleWithoutHashVersionBehavesAsVersion1(t *testing.T) {
	featuresJson := `{
    "feature": {"defaultValue": 0, "rules": [{"force": 1, "coverage": 0.5}]}
  }`

	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithAttributes(Attributes{"id": "user3"}),
	)
	require.NoError(t, err)

	result := client.EvalFeature(ctx, "feature")

	require.Equal(t, ForceResultSource, result.Source, "an unset hashVersion defaults to 1")
}

func TestForceRuleWithUnsupportedHashVersionSkipsTheRule(t *testing.T) {
	// hash() returns nil for any version other than 1 or 2, and the rule then has to be skipped so the
	// remaining rules still get their turn.
	featuresJson := `{
    "feature": {
      "defaultValue": 0,
      "rules": [
        {"force": 1, "range": [0, 1.0], "hashVersion": 3},
        {"force": 2}
      ]
    }
  }`

	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJson),
		WithAttributes(Attributes{"id": "user-1"}),
	)
	require.NoError(t, err)

	result := client.EvalFeature(ctx, "feature")

	require.EqualValues(t, 2, result.Value,
		"an unusable hash version must skip its own rule rather than abandon the whole feature")
	require.Equal(t, ForceResultSource, result.Source)
}
