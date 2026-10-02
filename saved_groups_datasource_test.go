package growthbook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSavedGroupsBackgroundUpdates covers real polling and SSE connections,
// including encrypted group-only updates, reconnects, and concurrent evaluation.
func TestSavedGroupsBackgroundUpdates(t *testing.T) {
	for _, transport := range []string{"poll", "sse"} {
		for _, encrypted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/encrypted=%v", transport, encrypted), func(t *testing.T) {
				testSavedGroupsBackgroundUpdates(t, transport, encrypted)
			})
		}
	}
}

func testSavedGroupsBackgroundUpdates(t *testing.T, transport string, encrypted bool) {
	t.Helper()
	const features = `{
		"flag":{"defaultValue":false,"rules":[{"condition":{"$savedGroup":{"id":"eligible"}},"force":true}]},
		"cycle":{"defaultValue":false,"rules":[{"condition":{"$savedGroup":{"id":"cycle"}},"force":true}]}
	}`
	groups := func(id string) string {
		return fmt.Sprintf(`{
			"members":{"type":"list","attributeKey":"other","values":[%q]},
			"eligible":{"type":"condition","condition":{"$and":[
				{"$savedGroup":{"id":"members","attributeKey":"id"}},
				{"$savedGroup":{"id":"members","attributeKey":"id"}}
			]}},
			"cycle":{"type":"condition","condition":{"$savedGroup":{"id":"cycle"}}}
		}`, id)
	}
	stamp := func(revision int) time.Time { return time.Unix(int64(revision), 0).UTC() }
	type payload struct {
		body     string
		revision int
	}
	makePayload := func(revision int, groupJSON string, initial bool) *payload {
		sections := map[string]any{"dateUpdated": stamp(revision)}
		if initial {
			if encrypted {
				sections["encryptedFeatures"] = encryptSavedGroupsTestJSON(t, features)
			} else {
				sections["features"] = json.RawMessage(features)
			}
		}
		if groupJSON == "" {
			// An empty fixture requests a malformed encrypted section.
			sections["encryptedSavedGroups"] = "bad.cipher"
		} else if encrypted {
			sections["encryptedSavedGroups"] = encryptSavedGroupsTestJSON(t, groupJSON)
		} else {
			sections["savedGroups"] = json.RawMessage(groupJSON)
		}
		body, err := json.Marshal(sections)
		require.NoError(t, err)
		return &payload{body: string(body), revision: revision}
	}

	var apiPayload atomic.Pointer[payload]
	apiPayload.Store(makePayload(1, groups("u1"), true))
	var apiCalls, connections, notModified atomic.Int32
	events := make(chan *payload) // A nil event closes the current SSE connection.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features/sdk-test":
			apiCalls.Add(1)
			current := apiPayload.Load()
			etag := fmt.Sprintf(`"%d"`, current.revision)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				notModified.Add(1)
				return
			}
			w.Header().Set("ETag", etag)
			w.Header().Set("x-sse-support", "enabled")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, current.body)
		case "/sub/sdk-test":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "retry: 5\n\n")
			w.(http.Flusher).Flush()
			connections.Add(1)
			for {
				select {
				case event := <-events:
					if event == nil {
						return
					}
					_, _ = fmt.Fprintf(w, "event: features\ndata: %s\n\n", event.body)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	option := WithPollDataSource(5 * time.Millisecond)
	if transport == "sse" {
		option = WithSseDataSource(WithSseMaxRetryInterval(5 * time.Millisecond))
	}
	client, err := NewClient(ctx,
		WithApiHost(server.URL), WithHttpClient(server.Client()), WithClientKey("sdk-test"),
		WithDecryptionKey(savedGroupsTestKey), WithAttributes(Attributes{"id": "u1"}),
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))), option,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.EnsureLoaded(ctx))
	child, err := client.WithAttributes(Attributes{"id": "u1"})
	require.NoError(t, err)
	require.Equal(t, true, child.EvalFeature(ctx, "flag").Value)
	if transport == "sse" {
		require.Eventually(t, func() bool { return connections.Load() == 1 }, 5*time.Second, time.Millisecond)
	}

	// Keep reading shared snapshots while the background transport replaces groups.
	readCtx, stopReaders := context.WithCancel(ctx)
	var readers sync.WaitGroup
	t.Cleanup(func() { stopReaders(); readers.Wait() })
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for readCtx.Err() == nil {
				if _, ok := child.EvalFeature(ctx, "flag").Value.(bool); !ok {
					t.Error("group-only update lost the feature definition")
					return
				}
				if child.EvalFeature(ctx, "cycle").Value != false {
					t.Error("cyclic reference matched during a background update")
					return
				}
				runtime.Gosched()
			}
		}()
	}
	sendEvent := func(event *payload) {
		t.Helper()
		select {
		case events <- event:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	waitRevision := func(revision int, want bool) {
		t.Helper()
		// Wait for application, not just HTTP delivery, even when the result is unchanged.
		require.Eventually(t, func() bool {
			return client.data.getDateUpdated().Equal(stamp(revision))
		}, 5*time.Second, time.Millisecond)
		require.Equal(t, want, client.EvalFeature(ctx, "flag").Value, "parent, revision %d", revision)
		require.Equal(t, want, child.EvalFeature(ctx, "flag").Value, "child, revision %d", revision)
	}
	for i, update := range []struct {
		name, groups string
		want         bool
	}{
		{"replace membership", groups("u2"), false},
		{"restore membership", groups("u1"), true},
		{"invalid encryption preserves membership", "", true},
		{"empty map clears membership", `{}`, false},
		{"recover after clearing", groups("u1"), true},
	} {
		revision := i + 2
		t.Logf("revision %d: %s", revision, update.name)
		body := makePayload(revision, update.groups, false)
		if transport == "poll" {
			apiPayload.Store(body)
		} else {
			sendEvent(body)
		}
		waitRevision(revision, update.want)
	}
	if transport == "sse" {
		// No event carries revision 7; only the reconnect's HTTP refresh can load it.
		apiPayload.Store(makePayload(7, groups("u2"), false))
		sendEvent(nil)
		waitRevision(7, false)
		require.GreaterOrEqual(t, apiCalls.Load(), int32(2))
		require.Eventually(t, func() bool { return connections.Load() >= 2 }, 5*time.Second, time.Millisecond)
		sendEvent(makePayload(8, groups("u1"), false))
		waitRevision(8, true)
	} else {
		// A subsequent poll proves the preceding 304 finished processing.
		before := notModified.Load()
		require.Eventually(t, func() bool { return notModified.Load() >= before+2 }, 5*time.Second, time.Millisecond)
		require.Equal(t, true, child.EvalFeature(ctx, "flag").Value)
		require.Equal(t, stamp(6), client.data.getDateUpdated())
	}
}
