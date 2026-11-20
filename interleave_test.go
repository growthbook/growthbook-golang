package growthbook

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const interleaveFlagJSON = `{"$interleave:search": {"defaultValue": "popular", "rules": [
	{"id": "$interleave:search", "force": {"interleave": true}, "hashAttribute": "id", "seed": "il-seed", "hashVersion": 2}
]}}`

const interleaveDefJSON = `{
	"trackingKey": "search-tk", "hashAttribute": "id", "seed": "il-seed",
	"variations": [
		{"key": "popular", "config": {"algorithm": "popular"}},
		{"key": "buyers-pick", "config": {"algorithm": "buyers-pick"}}
	]
}`

const interleaveDefsJSON = `{"search": ` + interleaveDefJSON + `}`

var interleaveTestRankers = []InterleaveRanker{
	{Key: "popular", Config: map[string]any{"algorithm": "popular"}},
	{Key: "buyers-pick", Config: map[string]any{"algorithm": "buyers-pick"}},
}

func interleaveClient(t *testing.T, features, interleavings string, opts ...ClientOption) *Client {
	t.Helper()
	client, err := NewClient(context.Background(), append([]ClientOption{withSilentTestLogger()}, opts...)...)
	require.NoError(t, err)
	require.NoError(t, client.UpdateFromApiResponseJSON(`{
		"features": `+features+`,
		"interleavings": `+interleavings+`,
		"dateUpdated": "2030-01-01T00:00:00Z"
	}`))
	return client
}

func TestInterleaveRankers(t *testing.T) {
	ctx := context.Background()

	t.Run("returns every interleaving's rankers in control-first order", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
		require.Equal(t, map[string][]InterleaveRanker{"search": interleaveTestRankers}, client.InterleaveRankers())
	})

	t.Run("returns every usable interleaving", func(t *testing.T) {
		flags := `{"$interleave:search": {}, "$interleave:home": {}, "$interleave:feed": {}}`
		defs := `{"search": ` + interleaveDefJSON + `, "home": ` + interleaveDefJSON + `, "feed": ` + interleaveDefJSON + `}`
		client := interleaveClient(t, flags, defs)
		require.Equal(t, map[string][]InterleaveRanker{
			"search": interleaveTestRankers, "home": interleaveTestRankers, "feed": interleaveTestRankers,
		}, client.InterleaveRankers())

		reduced, err := client.ReducePayload(ReducePayloadOptions{})
		require.NoError(t, err)
		var out map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(reduced, &out))
		require.JSONEq(t, defs, string(out["interleavings"]))
	})

	t.Run("returns the rankers for a single key", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
		require.Equal(t, interleaveTestRankers, client.InterleaveRankersForKey("search"))
		require.Nil(t, client.InterleaveRankersForKey("missing"))
	})

	t.Run("returns nothing before a payload loads and when it has no interleavings", func(t *testing.T) {
		client, err := NewClient(ctx)
		require.NoError(t, err)
		require.Empty(t, client.InterleaveRankers())
		require.NotNil(t, client.InterleaveRankers())
		require.Nil(t, client.InterleaveRankersForKey("search"))

		require.NoError(t, client.SetJSONFeatures(interleaveFlagJSON))
		require.Empty(t, client.InterleaveRankers())
	})

	t.Run("reads the payload off a child client", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
		child, err := client.WithAttributes(Attributes{"id": "u2"})
		require.NoError(t, err)
		require.Equal(t, map[string][]InterleaveRanker{"search": interleaveTestRankers}, child.InterleaveRankers())
	})

	t.Run("ignores targeting, since a pre-compute job has no user yet", func(t *testing.T) {
		targeted := `{"$interleave:search": {"defaultValue": "popular", "rules": [
			{"id": "$interleave:search", "condition": {"country": "US"}, "force": {"interleave": true}}
		]}}`
		client := interleaveClient(t, targeted, interleaveDefsJSON, WithAttributes(Attributes{"id": "u1"}))
		require.Equal(t, "popular", client.EvalFeature(ctx, "$interleave:search").Value)
		require.Equal(t, interleaveTestRankers, client.InterleaveRankersForKey("search"))
	})

	t.Run("skips a definition that isn't exactly two rankers", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, `{"search": {
			"trackingKey": "search-tk", "hashAttribute": "id", "seed": "il-seed",
			"variations": [{"key": "popular", "config": {"algorithm": "popular"}}]
		}}`)
		require.Empty(t, client.InterleaveRankers())
		require.Nil(t, client.InterleaveRankersForKey("search"))

		client = interleaveClient(t, interleaveFlagJSON, `{"search": {"variations": [
			{"key": "a", "config": {}}, {"key": "b", "config": {}}, {"key": "c", "config": {}}
		]}}`)
		require.Nil(t, client.InterleaveRankersForKey("search"))
	})

	t.Run("skips a null definition and one with no usable variations", func(t *testing.T) {
		flags := `{"$interleave:a": {}, "$interleave:b": {}, "$interleave:c": {}, "$interleave:d": {}, "$interleave:search": {}}`
		client := interleaveClient(t, flags, `{
			"a": null,
			"b": {"trackingKey": "b"},
			"c": {"variations": "junk"},
			"d": 5,
			"search": `+interleaveDefJSON+`
		}`)
		require.Equal(t, map[string][]InterleaveRanker{"search": interleaveTestRankers}, client.InterleaveRankers())
		for _, key := range []string{"a", "b", "c", "d"} {
			require.Nil(t, client.InterleaveRankersForKey(key))
		}
	})

	t.Run("skips a definition whose $interleave feature isn't in the payload", func(t *testing.T) {
		client := interleaveClient(t, `{"other": {"defaultValue": 1}}`, interleaveDefsJSON)
		require.Empty(t, client.InterleaveRankers())
		require.Nil(t, client.InterleaveRankersForKey("search"))
	})

	t.Run("hands back a copy, so a caller can't change what the client holds", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
		mine := client.InterleaveRankersForKey("search")
		mine[0].Key = "tampered"
		mine[0].Config["algorithm"] = "tampered"
		all := client.InterleaveRankers()
		all["search"][1].Key = "tampered"
		delete(all, "search")
		require.Equal(t, interleaveTestRankers, client.InterleaveRankersForKey("search"))
	})

	t.Run("copies nested config values and never returns a nil config", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, `{"search": {"variations": [
			{"key": "a", "config": {"weights": {"recency": 1}, "boosts": [{"n": 1}]}},
			{"key": "b"}
		]}}`)
		mine := client.InterleaveRankersForKey("search")
		mine[0].Config["weights"].(map[string]any)["recency"] = "tampered"
		mine[0].Config["boosts"].([]any)[0].(map[string]any)["n"] = "tampered"
		mine[1].Config["added"] = true
		require.Equal(t, []InterleaveRanker{
			{Key: "a", Config: map[string]any{"weights": map[string]any{"recency": 1.0}, "boosts": []any{map[string]any{"n": 1.0}}}},
			{Key: "b", Config: map[string]any{}},
		}, client.InterleaveRankersForKey("search"))
	})

	t.Run("copies definitions set from Go values", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, `{}`)
		require.NoError(t, client.UpdateFromApiResponse(&FeatureApiResponse{
			DateUpdated: time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC),
			Interleavings: InterleavingDefinitions{"search": {Variations: []InterleaveRanker{
				{Key: "a", Config: map[string]any{"weights": []float64{0.2, 0.8}}},
				{Key: "b", Config: map[string]any{}},
			}}},
		}))
		mine := client.InterleaveRankersForKey("search")
		mine[0].Config["weights"].([]any)[0] = 99.0
		require.Equal(t, []any{0.2, 0.8}, client.InterleaveRankersForKey("search")[0].Config["weights"])
	})

	t.Run("decodes a ranker's config into the caller's type", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
		var config struct {
			Algorithm string `json:"algorithm"`
		}
		require.NoError(t, client.InterleaveRankersForKey("search")[1].DecodeConfig(&config))
		require.Equal(t, "buyers-pick", config.Algorithm)

		var wrong struct {
			Algorithm int `json:"algorithm"`
		}
		require.Error(t, client.InterleaveRankersForKey("search")[1].DecodeConfig(&wrong))
	})
}

// reducedInterleavings returns the interleavings section Client.ReducePayload
// emits, or "" when it emits none.
func reducedInterleavings(t *testing.T, client *Client) string {
	t.Helper()
	reduced, err := client.ReducePayload(ReducePayloadOptions{})
	require.NoError(t, err)
	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(reduced, &out))
	return string(out["interleavings"])
}

func TestInterleavingPayloadSectionSemantics(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 16)
	withKey := WithDecryptionKey(base64.StdEncoding.EncodeToString(key))
	update := func(t *testing.T, client *Client, section string) {
		t.Helper()
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": `+interleaveFlagJSON+section+`,
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`))
	}

	t.Run("a features payload without the section clears the definitions", func(t *testing.T) {
		// What the server sends once the last interleaving stops
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"features": {"other": {"defaultValue": 1}},
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`))
		require.Empty(t, client.InterleaveRankers())
		reduced, err := client.ReducePayload(ReducePayloadOptions{})
		require.NoError(t, err)
		require.JSONEq(t, `{"dateUpdated": "2030-01-02T00:00:00Z", "features": {"other": {"defaultValue": 1}},
			"experiments": [], "savedGroups": {}}`, string(reduced))
	})

	t.Run("a payload without features or the section keeps the definitions", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
		require.NoError(t, client.UpdateFromApiResponseJSON(`{
			"savedGroups": {"g": ["a"]},
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`))
		require.Equal(t, interleaveTestRankers, client.InterleaveRankersForKey("search"))
		require.JSONEq(t, interleaveDefsJSON, reducedInterleavings(t, client))
	})

	t.Run("a payload with only the section counts as an update", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, `{}`)
		applied, _, err := client.applyApiResponseJSON(`{
			"interleavings": ` + interleaveDefsJSON + `,
			"dateUpdated": "2030-01-02T00:00:00Z"
		}`)
		require.NoError(t, err)
		require.True(t, applied)
		require.Equal(t, interleaveTestRankers, client.InterleaveRankersForKey("search"))
	})

	t.Run("an explicit empty or null section clears them, even without features", func(t *testing.T) {
		for _, section := range []string{`{}`, `null`, `[]`} {
			client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
			applied, _, err := client.applyApiResponseJSON(`{
				"interleavings": ` + section + `,
				"dateUpdated": "2030-01-02T00:00:00Z"
			}`)
			require.NoError(t, err)
			require.True(t, applied, section)
			require.Empty(t, client.InterleaveRankers(), section)
			require.JSONEq(t, `{}`, reducedInterleavings(t, client), section)
		}
	})

	t.Run("features that keep the flag but carry no section clear them", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
		update(t, client, "")
		require.Empty(t, client.InterleaveRankers())
		require.Empty(t, reducedInterleavings(t, client))
	})

	t.Run("a plaintext section wins over an undecodable encrypted one", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, `{}`, withKey)
		update(t, client, `, "interleavings": `+interleaveDefsJSON+`, "encryptedInterleavings": "not-encrypted"`)
		require.Equal(t, interleaveTestRankers, client.InterleaveRankersForKey("search"))
		require.JSONEq(t, interleaveDefsJSON, reducedInterleavings(t, client))
	})

	t.Run("reads an encrypted section", func(t *testing.T) {
		client, err := NewClient(context.Background(), withKey, withSilentTestLogger())
		require.NoError(t, err)
		update(t, client, `, "encryptedInterleavings": "`+encryptJSON(t, key, interleaveDefsJSON)+`"`)
		require.Equal(t, interleaveTestRankers, client.InterleaveRankersForKey("search"))
	})

	t.Run("a failed decryption drops the definitions, like the JS SDK", func(t *testing.T) {
		client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON, withKey)
		update(t, client, `, "encryptedInterleavings": "not-encrypted"`)
		require.Empty(t, client.InterleaveRankers())
	})

	t.Run("ReducePayload emits the decrypted section without the ciphertext", func(t *testing.T) {
		client, err := NewClient(context.Background(), withKey, withSilentTestLogger())
		require.NoError(t, err)
		update(t, client, `, "encryptedInterleavings": "`+encryptJSON(t, key, interleaveDefsJSON)+`"`)
		reduced, err := client.ReducePayload(ReducePayloadOptions{})
		require.NoError(t, err)
		var out map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(reduced, &out))
		require.NotContains(t, out, "encryptedInterleavings")
		require.JSONEq(t, interleaveDefsJSON, string(out["interleavings"]))
	})

	t.Run("ReducePayload drops a definition whose flag the filters removed", func(t *testing.T) {
		features := `{"$interleave:search": {"defaultValue": "popular", "metadata": {"projects": ["a"]}},
			"banner": {"defaultValue": 1, "metadata": {"projects": ["b"]}}}`
		client := interleaveClient(t, features, interleaveDefsJSON)
		for _, tc := range []struct {
			project string
			want    string
		}{{"a", interleaveDefsJSON}, {"b", `{}`}} {
			reduced, err := client.ReducePayload(ReducePayloadOptions{Include: PayloadFilters{Projects: []string{tc.project}}})
			require.NoError(t, err)
			var out map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(reduced, &out))
			require.JSONEq(t, tc.want, string(out["interleavings"]), tc.project)
		}
	})
}

func TestInterleaveRankersConcurrentWithUpdates(t *testing.T) {
	client := interleaveClient(t, interleaveFlagJSON, interleaveDefsJSON)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = client.UpdateFromApiResponseJSON(`{
				"features": ` + interleaveFlagJSON + `,
				"interleavings": ` + interleaveDefsJSON + `,
				"dateUpdated": "2030-01-02T00:00:00Z"
			}`)
		}
	}()
	for i := 0; i < 200; i++ {
		for _, ranker := range client.InterleaveRankers()["search"] {
			ranker.Config["touched"] = true
		}
		require.Len(t, client.InterleaveRankersForKey("search"), 2)
	}
	<-done
}
