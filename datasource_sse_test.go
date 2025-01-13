package growthbook

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSseDataSource(t *testing.T) {
	ctx := context.TODO()
	featuresJSON := []byte(`{
      "features": {
        "foo": {
          "defaultValue": "api"
        }
      },
      "experiments": [],
      "dateUpdated": "2000-05-01T00:00:12Z"
    }`)
	features := FeatureMap{"foo": &Feature{DefaultValue: "api"}}

	features2JSON := `{"features": { "foo": { "defaultValue": "SSE" } }, "experiments": [], "dateUpdated": "2000-05-02T00:00:12Z" }`
	features2 := FeatureMap{"foo": &Feature{DefaultValue: "SSE"}}

	t.Run("Update client data from sse data", func(t *testing.T) {
		ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 0))
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		require.Equal(t, features, client.data.features)
		require.Nil(t, err)
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, features2, client.data.features)
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Reconnect to server on connection break", func(t *testing.T) {
		ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 3))
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		time.Sleep(100 * time.Millisecond)
		require.Greater(t, ts.ssecount, 1)
		require.Equal(t, features2, client.data.features)
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Don't reconnect after closing client", func(t *testing.T) {
		ts := startSseServer(featuresJSON, sseResponse(features2JSON, 10*time.Millisecond, 3))
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithSseDataSource(),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		client.Close()
		old := ts.ssecount
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, old, ts.ssecount)
	})
}

type sseTestServer struct {
	http     *httptest.Server
	ssecount int
	apicount int
}

type sseResponseGen func(context.Context, http.ResponseWriter)

func startSseServer(apiResponse []byte, sseResponseGen sseResponseGen) *sseTestServer {
	var ts sseTestServer
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features/somekey":
			w.Header().Add("x-sse-support", "enabled")
			w.WriteHeader(http.StatusOK)
			w.Write(apiResponse)
			ts.apicount++
			return
		case "/sub/somekey":
			ts.ssecount++
			sseResponseGen(r.Context(), w)
		}
	}))
	return &ts
}

func sseResponse(response string, delay time.Duration, lim int) sseResponseGen {
	stream := []string{
		"retry: 10\n\n",
		"data:\n\n",
		fmt.Sprintf("id: 1\nevent: features\ndata: %s\n\n", response),
		"data:\n\n",
	}

	return func(ctx context.Context, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		ticker := time.NewTicker(delay)
		defer ticker.Stop()
		flusher := w.(http.Flusher)
		flusher.Flush()
		for count := 0; lim == 0 || count < lim; count++ {
			select {
			case <-ticker.C:
				if len(stream) > count {
					w.Write([]byte(stream[count]))
				} else {
					w.Write([]byte("data:\n\n"))
				}
				flusher.Flush()
				count++
			case <-ctx.Done():
				return
			}
		}
	}
}
