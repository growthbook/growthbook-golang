package growthbook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

type FeatureApiResponse struct {
	Status                     int                         `json:"status"`
	Features                   FeatureMap                  `json:"features"`
	DateUpdated                time.Time                   `json:"dateUpdated"`
	SavedGroups                condition.SavedGroups       `json:"savedGroups"`
	EncryptedFeatures          string                      `json:"encryptedFeatures"`
	ContextualBandits          ContextualBanditDefinitions `json:"contextualBandits"`
	EncryptedContextualBandits string                      `json:"encryptedContextualBandits"`
	SseSupport                 bool
	Etag                       string
}

const userAgent = "Growhthbook Go SDK client"

// maxDrainedErrorBody bounds how much of an error response is read so its connection can be pooled.
const maxDrainedErrorBody = 64 << 10

// FeatureApiError reports a non-success response from the features endpoint. It carries the status
// code so a caller can tell a request the host will never accept - a wrong client key - from one
// worth repeating.
type FeatureApiError struct {
	StatusCode int
}

func (e *FeatureApiError) Error() string {
	return fmt.Sprintf("Error loading features, code: %d", e.StatusCode)
}

// Retryable reports whether repeating the same request could plausibly succeed. A 4xx is the host's
// verdict on the request itself and will not change on its own; 408 and 429 are the two the status
// code marks as temporary.
func (e *FeatureApiError) Retryable() bool {
	switch e.StatusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}

	return e.StatusCode < 400 || e.StatusCode >= 500
}

func (c *Client) CallFeatureApi(ctx context.Context, etag string) (*FeatureApiResponse, error) {
	apiResp := FeatureApiResponse{}

	apiUrl := c.data.getApiUrl()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiUrl, nil)
	if err != nil {
		return nil, err
	}

	setReqHeaders(req, etag)
	resp, err := c.data.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	// Closed here rather than past the status checks below, which used to return without closing
	// the body at all.
	defer resp.Body.Close()

	apiResp.Status = resp.StatusCode
	apiResp.Etag = resp.Header.Get("etag")
	apiResp.SseSupport = resp.Header.Get("x-sse-support") == "enabled"

	if resp.StatusCode == 304 {
		return &apiResp, nil
	}

	if resp.StatusCode != 200 {
		// Drained before closing: a connection whose body was closed unread is dropped instead
		// of pooled, so an outage answered with an error page would have every retry paying for
		// a fresh connection. The cap keeps a large or hostile body from being read in full.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainedErrorBody))
		return &apiResp, &FeatureApiError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &apiResp, err
	}

	c.logger.InfoContext(ctx, "Loading features")
	err = json.Unmarshal(body, &apiResp)
	if err != nil {
		c.logger.ErrorContext(ctx, "Error parsing features response", "error", err)
		return &apiResp, err
	}

	return &apiResp, err
}

func setReqHeaders(req *http.Request, etag string) {
	req.Header.Set("User-Agent", userAgent)
	if etag != "" {
		req.Header.Add("If-None-Match", etag)
	}
}
