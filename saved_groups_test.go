package growthbook

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
	"github.com/tmaxmax/go-sse"
)

const savedGroupsTestKey = "Zvwv/+uhpFDznZ6SX28Yjg=="

// TestProgrammaticSavedGroups checks that native Go definitions behave like JSON
// payloads through both the client option and direct API-response updates.
func TestProgrammaticSavedGroups(t *testing.T) {
	groups := map[string]any{
		"legacy":    []string{"u1"},
		"list":      map[string]any{"type": "list", "attributeKey": "id", "values": []string{"u1"}},
		"condition": map[string]any{"type": "condition", "condition": map[string]any{"$savedGroup": "list"}},
		"cycle":     map[string]any{"type": "condition", "condition": map[string]any{"$savedGroup": "cycle"}},
		"malformed": map[string]any{"type": "list", "attributeKey": "id"},
	}
	ctx := context.Background()
	for _, tc := range []struct {
		name, cond string
		matches    bool
	}{
		{"legacy", `{"id":{"$inGroup":"legacy"}}`, true},
		{"list", `{"$savedGroup":"list"}`, true},
		{"condition", `{"$savedGroup":"condition"}`, true},
		{"cycle", `{"$savedGroup":"cycle"}`, false},
		{"malformed", `{"$savedGroup":"malformed"}`, false},
	} {
		for _, path := range []string{"option", "response", "json"} {
			t.Run(tc.name+"/"+path, func(t *testing.T) {
				features := fmt.Sprintf(`{"flag":{"defaultValue":false,"rules":[{"condition":%s,"force":true}]}}`, tc.cond)
				opts := []ClientOption{WithJsonFeatures(features)}
				if path == "option" {
					opts = append(opts, WithSavedGroups(groups))
				}
				client, err := NewClient(ctx, opts...)
				require.NoError(t, err)
				if path == "response" {
					require.NoError(t, client.UpdateFromApiResponse(&FeatureApiResponse{SavedGroups: groups}))
				} else if path == "json" {
					payload, err := json.Marshal(map[string]any{"savedGroups": groups})
					require.NoError(t, err)
					require.NoError(t, client.UpdateFromApiResponseJSON(string(payload)))
				}
				for _, id := range []string{"u1", "u2"} {
					child, err := client.WithAttributes(Attributes{"id": id})
					require.NoError(t, err)
					require.Equal(t, tc.matches && id == "u1", child.EvalFeature(ctx, "flag").Value)
				}
			})
		}
	}
	// Loading must not replace entries in the caller's map.
	require.IsType(t, map[string]any{}, groups["list"])
}

// TestWithSavedGroupsParsedDefinitions preserves legacy values and v2 groups
// that have already been decoded, including the distinction between nil and empty.
func TestWithSavedGroupsParsedDefinitions(t *testing.T) {
	var parsed condition.SavedGroups
	require.NoError(t, json.Unmarshal([]byte(`{"g":{"type":"condition","condition":{}}}`), &parsed))
	for _, groups := range []condition.SavedGroups{nil, {}, {"legacy": value.Arr("u1")}, parsed} {
		client, err := NewClient(context.Background(), WithSavedGroups(groups))
		require.NoError(t, err)
		require.Equal(t, groups, client.data.savedGroups)
	}
}

// TestProgrammaticSavedGroupsInvalidEncoding rejects values that cannot be JSON
// encoded without replacing previously loaded groups or partially applying updates.
func TestProgrammaticSavedGroupsInvalidEncoding(t *testing.T) {
	ctx := context.Background()
	invalid := map[string]any{"bad": make(chan int)}
	_, err := NewClient(ctx, WithSavedGroups(invalid))
	var unsupported *json.UnsupportedTypeError
	require.ErrorAs(t, err, &unsupported)
	client, err := NewClient(ctx, WithSavedGroups(map[string]any{"legacy": []string{"u1"}}))
	require.NoError(t, err)
	previous := client.data.savedGroups
	err = client.UpdateFromApiResponse(&FeatureApiResponse{
		SavedGroups: invalid,
		Features:    FeatureMap{"new": &Feature{DefaultValue: true}},
	})
	require.ErrorAs(t, err, &unsupported)
	require.Equal(t, previous, client.data.savedGroups)
	require.NotContains(t, client.data.features, "new")
}

func encryptSavedGroupsTestJSON(t *testing.T, plaintext string) string {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(savedGroupsTestKey)
	require.NoError(t, err)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	// A fixed IV makes fixtures reproducible; this helper is only for tests.
	iv := bytes.Repeat([]byte{1}, aes.BlockSize)
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append([]byte(plaintext), bytes.Repeat([]byte{byte(padding)}, padding)...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(padded, padded)
	return base64.StdEncoding.EncodeToString(iv) + "." + base64.StdEncoding.EncodeToString(padded)
}

// TestEncryptedSavedGroupsLoadingPaths checks that legacy and v2 encrypted
// groups match plaintext evaluation through manual, refresh, polling, and SSE loads.
func TestEncryptedSavedGroupsLoadingPaths(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, groups, cond string }{
		// Legacy arrays remain usable through the attribute-level $inGroup operator.
		{"legacy", `{"beta":["u1"]}`, `{"id":{"$inGroup":"beta"}}`},
		// A v2 condition group references a list group through top-level $savedGroup.
		{"v2", `{"beta":{"type":"list","attributeKey":"id","values":["u1"]},"eligible":{"type":"condition","condition":{"$savedGroup":"beta"}}}`, `{"$savedGroup":"eligible"}`},
	} {
		// Each loading path must match plaintext evaluation for members and nonmembers.
		for _, transport := range []string{"manual", "refresh", "poll", "sse"} {
			t.Run(tc.name+"/"+transport, func(t *testing.T) {
				features := `{"flag":{"defaultValue":false,"rules":[{"condition":` + tc.cond + `,"force":true}]}}`
				plainJSON := `{"features":` + features + `,"savedGroups":` + tc.groups + `}`
				payload, err := json.Marshal(map[string]any{
					"features":             map[string]any{},
					"encryptedFeatures":    encryptSavedGroupsTestJSON(t, features),
					"encryptedSavedGroups": encryptSavedGroupsTestJSON(t, tc.groups),
				})
				require.NoError(t, err)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(payload)
				}))
				defer server.Close()
				client, err := NewClient(ctx, WithDecryptionKey(savedGroupsTestKey), WithApiHost(server.URL), WithClientKey("sdk-test"))
				require.NoError(t, err)
				plain, err := NewClient(ctx)
				require.NoError(t, err)
				require.NoError(t, plain.UpdateFromApiResponseJSON(plainJSON))
				switch transport {
				case "manual":
					require.NoError(t, client.UpdateFromApiResponseJSON(string(payload)))
				case "refresh":
					require.NoError(t, client.RefreshFeatures(ctx))
				case "poll":
					require.NoError(t, newPollDataSource(client, time.Minute).loadData(ctx))
				case "sse":
					newSseDataSource(client).processEvent(sse.Event{Data: string(payload)})
				}
				for _, id := range []string{"u1", "u2"} {
					actual, err := client.WithAttributes(Attributes{"id": id})
					require.NoError(t, err)
					expected, err := plain.WithAttributes(Attributes{"id": id})
					require.NoError(t, err)
					require.Equal(t, id == "u1", actual.EvalFeature(ctx, "flag").Value)
					require.Equal(t, expected.EvalFeature(ctx, "flag"), actual.EvalFeature(ctx, "flag"))
				}
			})
		}
	}
}

// TestEncryptedSavedGroupsUpdates checks preservation, clearing, precedence,
// and failure handling across updates, including shared child state and stale data.
func TestEncryptedSavedGroupsUpdates(t *testing.T) {
	ctx := context.Background()
	initial := `{"features":{"flag":{"defaultValue":false,"rules":[{"condition":{"$savedGroup":"g"},"force":true}]}},"savedGroups":{"g":{"type":"list","attributeKey":"id","values":["u1"]}}}`
	for _, tc := range []struct {
		name, key, plaintext, encrypted string
		want                            bool
	}{
		// Missing/null sections preserve the loaded group; an explicit empty map clears it.
		{name: "absent preserves", key: savedGroupsTestKey, want: true},
		{name: "null plaintext preserves", key: savedGroupsTestKey, plaintext: `null`, want: true},
		{name: "null encrypted preserves", key: savedGroupsTestKey, encrypted: encryptSavedGroupsTestJSON(t, `null`), want: true},
		{name: "empty encrypted clears", key: savedGroupsTestKey, encrypted: encryptSavedGroupsTestJSON(t, `{}`)},
		// Successfully decrypted groups take precedence over the plaintext section.
		{name: "encrypted wins over plaintext", key: savedGroupsTestKey, plaintext: `{"g":{"type":"list","attributeKey":"id","values":["u1"]}}`, encrypted: encryptSavedGroupsTestJSON(t, `{}`)},
		// Decryption or decoding failures leave the previous groups intact.
		{name: "missing key preserves", encrypted: encryptSavedGroupsTestJSON(t, `{}`), want: true},
		{name: "wrong key preserves", key: "AAAAAAAAAAAAAAAAAAAAAA==", encrypted: encryptSavedGroupsTestJSON(t, `{}`), want: true},
		{name: "invalid encoding preserves", key: savedGroupsTestKey, encrypted: "bad-blob", want: true},
		{name: "invalid JSON preserves", key: savedGroupsTestKey, encrypted: encryptSavedGroupsTestJSON(t, `{`), want: true},
		{name: "invalid map preserves", key: savedGroupsTestKey, encrypted: encryptSavedGroupsTestJSON(t, `[]`), want: true},
		{name: "invalid ciphertext length preserves", key: savedGroupsTestKey, encrypted: "AQEBAQEBAQEBAQEBAQEBAQ==.AQ==", want: true},
		// On failure, a supplied plaintext section still applies (here, clearing groups).
		{name: "plaintext fallback on decryption failure", key: savedGroupsTestKey, plaintext: `{}`, encrypted: "bad-blob"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(ctx, WithDecryptionKey(tc.key), WithAttributes(Attributes{"id": "u1"}))
			require.NoError(t, err)
			require.NoError(t, client.UpdateFromApiResponseJSON(initial))
			child, err := client.WithAttributes(Attributes{"id": "u1"})
			require.NoError(t, err)
			update := map[string]any{"encryptedSavedGroups": tc.encrypted, "dateUpdated": "2026-09-17T00:00:00Z"}
			if tc.plaintext != "" {
				update["savedGroups"] = json.RawMessage(tc.plaintext)
			}
			body, err := json.Marshal(update)
			require.NoError(t, err)
			require.NoError(t, client.UpdateFromApiResponseJSON(string(body)))
			// Parent and child share the updated groups and accept the payload timestamp.
			require.Equal(t, tc.want, client.EvalFeature(ctx, "flag").Value)
			require.Equal(t, tc.want, child.EvalFeature(ctx, "flag").Value)
			require.Equal(t, time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC), client.data.getDateUpdated())
		})
	}

	t.Run("stale encrypted update is ignored", func(t *testing.T) {
		client, err := NewClient(ctx, WithDecryptionKey(savedGroupsTestKey), WithAttributes(Attributes{"id": "u1"}))
		require.NoError(t, err)
		require.NoError(t, client.UpdateFromApiResponseJSON(initial))
		require.NoError(t, client.UpdateFromApiResponse(&FeatureApiResponse{DateUpdated: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)}))
		require.NoError(t, client.UpdateFromApiResponse(&FeatureApiResponse{EncryptedSavedGroups: encryptSavedGroupsTestJSON(t, `{}`)}))
		require.True(t, client.EvalFeature(ctx, "flag").Value.(bool))
	})
	t.Run("invalid groups do not block features", func(t *testing.T) {
		client, err := NewClient(ctx, WithDecryptionKey(savedGroupsTestKey))
		require.NoError(t, err)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{"features":{"new":{"defaultValue":true}},"encryptedSavedGroups":"bad-blob"}`))
		require.Equal(t, true, client.EvalFeature(ctx, "new").Value)
	})
}

// TestSavedGroupsConcurrentUpdates checks that concurrent updates and child
// evaluations preserve shared groups while keeping cycle tracking branch-local.
func TestSavedGroupsConcurrentUpdates(t *testing.T) {
	// Repeated sibling references and concurrent child evaluations must not share
	// cycle-tracking state, even while the shared group definitions are replaced.
	ctx := context.Background()
	client, err := NewClient(ctx, WithDecryptionKey(savedGroupsTestKey), WithAttributes(Attributes{"id": "u1"}), WithJsonFeatures(`{"flag":{"defaultValue":false,"rules":[{"condition":{"$and":[{"$savedGroup":"g"},{"$savedGroup":"g"}]},"force":true}]}}`))
	require.NoError(t, err)
	response := FeatureApiResponse{EncryptedSavedGroups: encryptSavedGroupsTestJSON(t, `{"g":{"type":"condition","condition":{"id":"u1"}}}`)}
	require.NoError(t, client.UpdateFromApiResponse(&response))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		child, err := client.WithAttributes(Attributes{"id": "u1"})
		require.NoError(t, err)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if j%10 == 0 {
					if err := child.UpdateFromApiResponse(&response); err != nil {
						t.Error(err)
					}
				}
				if child.EvalFeature(ctx, "flag").Value != true {
					t.Error("concurrent evaluation lost saved-group state")
				}
			}
		}()
	}
	wg.Wait()
}
