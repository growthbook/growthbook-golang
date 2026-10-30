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

// A malformed part fails the condition only when evaluation reaches it, as in
// the JS SDK, where it throws. An earlier passing $or branch decides first.
func TestMalformedPartFailsOnlyWhenReached(t *testing.T) {
	features := `{"f": {"defaultValue": "default", "rules": [
		{"condition": {"$or": [{"id": "123"}, {"$not": null}]}, "force": "matched"}
	]}}`
	tests := []struct {
		id   string
		want string
	}{
		{"123", "matched"},
		{"999", "default"},
	}
	for _, tt := range tests {
		client, err := NewClient(context.TODO(), withSilentTestLogger(), WithAttributes(Attributes{"id": tt.id}))
		require.NoError(t, err)
		require.NoError(t, client.SetJSONFeatures(features))
		require.Equal(t, tt.want, client.EvalFeature(context.TODO(), "f").Value, "id %s", tt.id)
	}
}

// A malformed part fails the whole condition, so negating it doesn't match.
func TestMalformedPartUnderNotFails(t *testing.T) {
	client, err := NewClient(context.TODO(), withSilentTestLogger(), WithAttributes(Attributes{"id": "999"}))
	require.NoError(t, err)
	require.NoError(t, client.SetJSONFeatures(`{"f": {"defaultValue": "default", "rules": [
		{"condition": {"$not": {"$or": [{"id": "123"}, {"$not": null}]}}, "force": "negated"}
	]}}`))
	require.Equal(t, "default", client.EvalFeature(context.TODO(), "f").Value)
}

// The reduced payload agrees with the full one for a condition with a
// malformed $or branch after a passing one.
func TestReducePayloadAgreesOnMalformedLaterBranch(t *testing.T) {
	payload := `{"features": {"f": {"defaultValue": "default", "rules": [
		{"condition": {"$or": [{"id": "123"}, {"$not": null}]}, "force": "matched"}
	]}}}`
	attrs := Attributes{"id": "123"}
	reduced, err := ReducePayload([]byte(payload), attrs, ReducePayloadOptions{})
	require.NoError(t, err)
	for _, p := range []string{payload, string(reduced)} {
		client, err := NewClient(context.TODO(), withSilentTestLogger(), WithAttributes(attrs))
		require.NoError(t, err)
		require.NoError(t, client.UpdateFromApiResponseJSON(p))
		require.Equal(t, "matched", client.EvalFeature(context.TODO(), "f").Value, p)
	}
}
