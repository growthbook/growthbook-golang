package growthbook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFeatureApiErrorSeparatesTemporaryFailuresFromVerdicts(t *testing.T) {
	for _, code := range []int{http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusRequestTimeout, http.StatusTooManyRequests} {
		require.True(t, (&FeatureApiError{StatusCode: code}).Retryable(), "code %d", code)
	}

	for _, code := range []int{http.StatusBadRequest, http.StatusUnauthorized,
		http.StatusForbidden, http.StatusNotFound} {
		require.False(t, (&FeatureApiError{StatusCode: code}).Retryable(), "code %d", code)
	}

	require.EqualError(t, &FeatureApiError{StatusCode: http.StatusNotFound},
		"Error loading features, code: 404", "the wording predates the type and callers may match it")
}

func TestAnErrorResponseKeepsItsConnectionPooled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"staged failure"}`)
	}))
	defer server.Close()

	client, err := NewClient(ctx,
		WithApiHost(server.URL),
		WithClientKey("somekey"),
		WithHttpClient(&http.Client{}), // its own pool, so sibling tests cannot colour the result
	)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	var reused int

	for attempt := 0; attempt < 5; attempt++ {
		var pooled bool

		trace := &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) { pooled = info.Reused },
		}

		_, err := client.CallFeatureApi(httptrace.WithClientTrace(ctx, trace), "")
		require.Error(t, err)

		if pooled {
			reused++
		}
	}

	require.Positive(t, reused,
		"an error body closed unread has its connection dropped, so every retry pays for a new one")
}

func TestJsonUnmarshaling(t *testing.T) {
	apiJson := `{
      "features": {
        "foo": {
          "defaultValue": "api"
        }
      },
      "experiments": [],
      "dateUpdated": "2000-05-01T00:00:12Z"
    }`
	var apiResp FeatureApiResponse
	err := json.Unmarshal([]byte(apiJson), &apiResp)
	require.Nil(t, err)
	require.Equal(t,
		FeatureApiResponse{
			Features:    FeatureMap{"foo": &Feature{DefaultValue: "api"}},
			DateUpdated: time.Date(2000, time.May, 1, 0, 0, 12, 0, time.UTC),
		},
		apiResp)
}
