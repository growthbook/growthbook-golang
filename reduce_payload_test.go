package growthbook

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"testing"

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
