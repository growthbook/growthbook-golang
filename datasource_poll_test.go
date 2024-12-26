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

func TestPollingDataSource(t *testing.T) {
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

	t.Run("Update client data from valid server response", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(100*time.Millisecond),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		require.Nil(t, err)
		require.Equal(t, features, client.data.features)
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Closing client stops data loading", func(t *testing.T) {
		ts := startServer(http.StatusOK, featuresJSON)
		defer ts.http.Close()
		client, _ := NewClient(ctx,
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(10*time.Millisecond),
		)
		client.EnsureLoaded(ctx)
		client.Close()
		require.True(t, ts.count > 0)
		ts.count = 0
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, 0, ts.count)
	})

	t.Run("EnsureLoaded returns error on invalid server response", func(t *testing.T) {
		ts := startServer(http.StatusNotFound, []byte(""))
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(100*time.Millisecond),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		require.Error(t, fmt.Errorf("Error loading from server, code: %d,", http.StatusNotFound), err)
		err = client.Close()
		require.Nil(t, err)
	})

	t.Run("Use etags for requests if present", func(t *testing.T) {
		ts := startEtagServer(featuresJSON)
		defer ts.http.Close()
		client, err := NewClient(ctx,
			WithHttpClient(ts.http.Client()),
			WithApiHost(ts.http.URL),
			WithClientKey("somekey"),
			WithPollDataSource(10*time.Millisecond),
		)
		require.Nil(t, err)
		err = client.EnsureLoaded(ctx)
		require.Nil(t, err)
		require.Equal(t, features, client.data.features)
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, features, client.data.features)
		require.True(t, ts.count > 2)
		require.Equal(t, ts.count-1, ts.etagCount)
	})
}

type testServer struct {
	http      *httptest.Server
	count     int
	etagCount int
}

func startServer(code int, response []byte) *testServer {
	var ts testServer
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.count++
		w.WriteHeader(code)
		_, _ = w.Write(response)
	}))
	return &ts
}

func startEtagServer(response []byte) *testServer {
	var ts testServer
	etag := `W/"SOME_ETAG_VALUE"`
	ts.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.count++
		if r.Header.Get("If-None-Match") == etag {
			ts.etagCount++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("etag", etag)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(response)
	}))
	return &ts
}
