package growthbook

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/stretchr/testify/require"
)

func encryptJSON(t *testing.T, key []byte, plaintext string) string {
	t.Helper()
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	return encryptCryptoTestBytes(t, key, append([]byte(plaintext), bytes.Repeat([]byte{byte(padding)}, padding)...))
}

func TestClientReducePayloadUsesDecryptedResponseJSON(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 16)
	features := `{"f": {"defaultValue": 0, "metadata": {"projects": ["p1"]},
		"rules": [{"condition": {"id": "123"}, "force": 1}, {"condition": {"country": "US"}, "force": 2}]}}`
	client, err := NewClient(context.TODO(),
		WithDecryptionKey(base64.StdEncoding.EncodeToString(key)),
		WithAttributes(Attributes{"id": "123"}),
		withSilentTestLogger(),
	)
	require.NoError(t, err)
	require.NoError(t, client.UpdateFromApiResponseJSON(`{"dateUpdated": "2026-10-04T00:00:00Z",
		"encryptedFeatures": "`+encryptJSON(t, key, features)+`"}`))

	res, err := client.ReducePayload(ReducePayloadOptions{Include: PayloadFilters{Projects: []string{"p1"}}})
	require.NoError(t, err)
	require.JSONEq(t, `{"dateUpdated": "2026-10-04T00:00:00Z",
		"features": {"f": {"defaultValue": 0, "metadata": {"projects": ["p1"]},
			"rules": [{"force": 1}, {"condition": {"country": "US"}, "force": 2}]}},
		"experiments": [], "savedGroups": {}}`, string(res))

	res, err = client.ReducePayload(ReducePayloadOptions{Include: PayloadFilters{Projects: []string{"other"}}})
	require.NoError(t, err)
	require.JSONEq(t, `{"dateUpdated": "2026-10-04T00:00:00Z", "features": {}, "experiments": [], "savedGroups": {}}`, string(res))
}

func TestClientReducePayloadKeepsSectionsAPartialUpdateOmits(t *testing.T) {
	client, err := NewClient(context.TODO(), withSilentTestLogger())
	require.NoError(t, err)
	require.NoError(t, client.UpdateFromApiResponseJSON(`{"features": {"f": {"defaultValue": 1}}, "savedGroups": {"g": ["f"]}}`))
	require.NoError(t, client.UpdateFromApiResponseJSON(`{"contextualBandits": {}}`))

	res, err := client.ReducePayload(ReducePayloadOptions{})
	require.NoError(t, err)
	require.JSONEq(t, `{"features": {"f": {"defaultValue": 1}}, "experiments": [], "savedGroups": {}, "contextualBandits": {}}`, string(res))
}

func TestClientReducePayloadNeedsJSON(t *testing.T) {
	client, err := NewClient(context.TODO(), WithFeatures(FeatureMap{"f": &Feature{DefaultValue: 1}}), withSilentTestLogger())
	require.NoError(t, err)
	_, err = client.ReducePayload(ReducePayloadOptions{})
	require.ErrorIs(t, err, ErrNoPayloadJSON)

	require.NoError(t, client.SetJSONFeatures(`{"f": {"defaultValue": 2}}`))
	res, err := client.ReducePayload(ReducePayloadOptions{})
	require.NoError(t, err)
	require.JSONEq(t, `{"features": {"f": {"defaultValue": 2}}, "experiments": [], "savedGroups": {}}`, string(res))
}

func TestPruneConditionReturnsMalformedConditionAsWritten(t *testing.T) {
	cond := map[string]any{"$not": nil, "id": "123"}
	res, err := PruneCondition(cond, Attributes{"id": "123"}, nil)
	require.NoError(t, err)
	require.Equal(t, cond, res)
}

// A bandit context whose condition isn't an object falls back to the default
// weights in evaluation, so reduction keeps it as written rather than turning
// it into a catch-all. A missing or null condition still becomes {}.
func TestReducePayloadKeepsNonObjectBanditConditionAsWritten(t *testing.T) {
	payload := `{"features": {"f": {"defaultValue": "a", "rules": [{"key": "exp", "variations": ["a", "b"],
		"contextualBanditRef": "cb", "hashAttribute": "id"}]}},
		"contextualBandits": {"cb": {"contexts": [
			{"leafId": 1, "condition": 5, "weights": [1, 0]},
			{"leafId": 2, "condition": null, "weights": [0, 1]}
		]}}}`
	res, err := ReducePayload([]byte(payload), Attributes{"id": "u"}, ReducePayloadOptions{})
	require.NoError(t, err)
	require.JSONEq(t, `{"features": {"f": {"defaultValue": "a", "rules": [{"key": "exp", "variations": ["a", "b"],
		"contextualBanditRef": "cb", "hashAttribute": "id"}]}},
		"experiments": [], "savedGroups": {},
		"contextualBandits": {"cb": {"contexts": [
			{"leafId": 1, "condition": 5, "weights": [1, 0]},
			{"leafId": 2, "condition": {}, "weights": [0, 1]}
		]}}}`, string(res))
}

// Numeric custom field filters accept any Go number type.
func TestReducePayloadNumericFilterValues(t *testing.T) {
	payload := `{"features": {
		"one": {"defaultValue": 1, "metadata": {"customFields": {"level": 1}}},
		"half": {"defaultValue": 2, "metadata": {"customFields": {"level": 0.5}}}
	}}`
	for _, want := range []any{int8(1), int16(1), int32(1), int64(1), uint(1), uint8(1), uint16(1), uint32(1), uint64(1), float32(1), json.Number("1")} {
		res, err := ReducePayload([]byte(payload), nil, ReducePayloadOptions{
			Include: PayloadFilters{CustomFields: map[string][]any{"level": {want}}},
		})
		require.NoError(t, err)
		require.JSONEq(t, `{"features": {"one": {"defaultValue": 1, "metadata": {"customFields": {"level": 1}}}},
			"experiments": [], "savedGroups": {}}`, string(res), "%T", want)

		res, err = ReducePayload([]byte(payload), nil, ReducePayloadOptions{
			Exclude: PayloadFilters{CustomFields: map[string][]any{"level": {want}}},
		})
		require.NoError(t, err)
		require.JSONEq(t, `{"features": {"half": {"defaultValue": 2, "metadata": {"customFields": {"level": 0.5}}}},
			"experiments": [], "savedGroups": {}}`, string(res), "%T", want)
	}
}

// The stored payload JSON follows each section's update decision, so a
// reduced payload never carries data evaluation has dropped.
func TestClientReducePayloadFollowsSectionDecisions(t *testing.T) {
	t.Run("explicit null contextual bandits clear them", func(t *testing.T) {
		client, err := NewClient(context.TODO(), withSilentTestLogger())
		require.NoError(t, err)
		features := `{"f": {"defaultValue": 0, "rules": [{"key": "exp", "variations": [0, 1], "contextualBanditRef": "cb"}]}}`
		require.NoError(t, client.UpdateFromApiResponseJSON(`{"features": `+features+`,
			"contextualBandits": {"cb": {"contexts": []}}}`))
		require.NoError(t, client.UpdateFromApiResponseJSON(`{"contextualBandits": null}`))
		res, err := client.ReducePayload(ReducePayloadOptions{})
		require.NoError(t, err)
		require.JSONEq(t, `{"features": `+features+`, "experiments": [], "savedGroups": {},
			"contextualBandits": {}}`, string(res))
	})

	t.Run("SetJSONFeatures with null clears features", func(t *testing.T) {
		client, err := NewClient(context.TODO(), withSilentTestLogger())
		require.NoError(t, err)
		require.NoError(t, client.SetJSONFeatures(`{"f": {"defaultValue": 1}}`))
		require.NoError(t, client.SetJSONFeatures(`null`))
		res, err := client.ReducePayload(ReducePayloadOptions{})
		require.NoError(t, err)
		require.JSONEq(t, `{"features": {}, "experiments": [], "savedGroups": {}}`, string(res))
	})

	t.Run("rejected encrypted saved groups keep the accepted ones", func(t *testing.T) {
		key := bytes.Repeat([]byte{3}, 16)
		client, err := NewClient(context.TODO(),
			WithDecryptionKey(base64.StdEncoding.EncodeToString(key)),
			WithAttributes(Attributes{"id": "1"}),
			withSilentTestLogger())
		require.NoError(t, err)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": {"f": {"defaultValue": false, "rules": [{"condition": {"id": {"$inGroup": "g"}}, "force": true}]}},
			"savedGroups": {"g": ["1"]},
			"encryptedSavedGroups": "`+encryptJSON(t, key, `[]`)+`"}`))
		require.Equal(t, true, client.EvalFeature(context.TODO(), "f").Value)
		res, err := client.ReducePayload(ReducePayloadOptions{})
		require.NoError(t, err)
		require.JSONEq(t, `{"features": {"f": {"defaultValue": false, "rules": [{"force": true}]}},
			"experiments": [], "savedGroups": {}}`, string(res))
	})
}

// Saved groups and contextual bandits set in Go code are part of the reduced
// payload, since evaluation uses them.
func TestClientReducePayloadIncludesDefinitionsSetInCode(t *testing.T) {
	groups := condition.SavedGroups{"g": map[string]any{"type": "list", "attributeKey": "id", "values": []any{"1"}}}
	client, err := NewClient(context.TODO(), WithSavedGroups(groups), withSilentTestLogger())
	require.NoError(t, err)
	features := `{
		"f": {"defaultValue": false, "rules": [{"condition": {"$savedGroup": {"id": "g"}}, "force": true}]},
		"b": {"defaultValue": 0, "rules": [{"key": "exp", "variations": [0, 1], "contextualBanditRef": "cb"}]}
	}`
	require.NoError(t, client.SetJSONFeatures(features))
	require.NoError(t, client.SetContextualBandits(mustBanditDefs(t, `{"cb": {"contexts": [{"leafId": 1, "condition": {}, "weights": [0, 1]}]}}`)))

	res, err := client.ReducePayload(ReducePayloadOptions{})
	require.NoError(t, err)
	require.JSONEq(t, `{"features": `+features+`,
		"experiments": [],
		"savedGroups": {"g": {"type": "list", "attributeKey": "id", "values": ["1"]}},
		"contextualBandits": {"cb": {"banditVersion": null, "contexts": [{"leafId": 1, "condition": {}, "weights": [0, 1]}]}}}`, string(res))
}

// Legacy saved groups keep their exact JSON shape in the reduced payload: an
// empty list stays [] and null members stay null, whether they came from a
// payload or were set in Go code.
func TestClientReducePayloadKeepsLegacySavedGroupShapes(t *testing.T) {
	features := `{"f": {"defaultValue": false, "rules": [
		{"condition": {"id": {"$notInGroup": "empty"}, "x": {"$inGroup": "nulls"}}, "force": true}
	]}}`
	want := `{"features": ` + features + `, "experiments": [],
		"savedGroups": {"empty": [], "nulls": [null, "a"]}}`

	client, err := NewClient(context.TODO(), withSilentTestLogger())
	require.NoError(t, err)
	require.NoError(t, client.UpdateFromApiResponseJSON(`{"features": `+features+`,
		"savedGroups": {"empty": [], "nulls": [null, "a"]}}`))
	res, err := client.ReducePayload(ReducePayloadOptions{})
	require.NoError(t, err)
	require.JSONEq(t, want, string(res), "from a payload")

	groups := condition.SavedGroups{"empty": []any{}, "nulls": []any{nil, "a"}}
	client, err = NewClient(context.TODO(), WithSavedGroups(groups), withSilentTestLogger())
	require.NoError(t, err)
	require.NoError(t, client.SetJSONFeatures(features))
	res, err = client.ReducePayload(ReducePayloadOptions{})
	require.NoError(t, err)
	require.JSONEq(t, want, string(res), "set in code")
}
