package growthbook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// A rule whose condition is malformed is skipped instead of failing the whole
// payload, as in the JS SDK.
func TestMalformedRuleConditionSkipsRule(t *testing.T) {
	client, err := NewClient(context.TODO(), withSilentTestLogger())
	require.NoError(t, err)
	require.NoError(t, client.SetJSONFeatures(`{"f": {"defaultValue": "default", "rules": [
		{"condition": {"$not": null}, "force": "malformed"},
		{"force": "next"}
	]}}`))
	res := client.EvalFeature(context.TODO(), "f")
	require.Equal(t, "next", res.Value)
}
