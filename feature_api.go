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
	EncryptedSavedGroups       string                      `json:"encryptedSavedGroups,omitempty"`
	ContextualBandits          ContextualBanditDefinitions `json:"contextualBandits"`
	EncryptedContextualBandits string                      `json:"encryptedContextualBandits"`
	SseSupport                 bool
	Etag                       string
	// raw is the JSON the response was decoded from, kept for ReducePayload
	raw json.RawMessage
}

// UnmarshalJSON decodes a response and keeps its JSON, so the client can
// reduce the payload with metadata and keys this SDK doesn't model.
func (r *FeatureApiResponse) UnmarshalJSON(data []byte) error {
	type plain FeatureApiResponse
	var resp plain
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	resp.Status, resp.SseSupport, resp.Etag = r.Status, r.SseSupport, r.Etag
	*r = FeatureApiResponse(resp)
	r.raw = append(json.RawMessage(nil), data...)
	return nil
}

// FeatureApiError reports a features API response with a status other than
// 200 or 304.
type FeatureApiError struct {
	StatusCode int
}

func (e *FeatureApiError) Error() string {
	return fmt.Sprintf("Error loading features, code: %d", e.StatusCode)
}

const userAgent = "Growhthbook Go SDK client"

func (c *Client) CallFeatureApi(ctx context.Context, etag string) (*FeatureApiResponse, error) {
	return c.callFeatureApi(ctx, etag, RefreshDefault)
}

// callFeatureApi fetches the payload once.
func (c *Client) callFeatureApi(ctx context.Context, etag string, mode RefreshMode) (*FeatureApiResponse, error) {
	req, err := c.newFeatureApiRequest(ctx, etag, mode)
	if err != nil {
		return nil, err
	}
	resp, err := c.data.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return c.readFeatureApiResponse(ctx, resp)
}

// newFeatureApiRequest builds the features request. A forced one skips the
// ETag and asks caches between the SDK and the API for a fresh response.
func (c *Client) newFeatureApiRequest(ctx context.Context, etag string, mode RefreshMode) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.data.getApiUrl(), nil)
	if err != nil {
		return nil, err
	}
	if mode == RefreshForce {
		setReqHeaders(req, "")
		req.Header.Set("Cache-Control", "no-cache")
	} else {
		setReqHeaders(req, etag)
	}
	return req, nil
}

func (c *Client) readFeatureApiResponse(ctx context.Context, resp *http.Response) (*FeatureApiResponse, error) {
	apiResp := FeatureApiResponse{
		Status:     resp.StatusCode,
		Etag:       resp.Header.Get("etag"),
		SseSupport: resp.Header.Get("x-sse-support") == "enabled",
	}
	switch resp.StatusCode {
	case http.StatusNotModified:
		return &apiResp, nil
	case http.StatusOK:
	default:
		return &apiResp, &FeatureApiError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &apiResp, err
	}
	c.logger.InfoContext(ctx, "Loading features")
	if err := json.Unmarshal(body, &apiResp); err != nil {
		c.logger.ErrorContext(ctx, "Error parsing features response", "error", err)
		return &apiResp, err
	}
	return &apiResp, nil
}

func setReqHeaders(req *http.Request, etag string) {
	req.Header.Set("User-Agent", userAgent)
	if etag != "" {
		req.Header.Add("If-None-Match", etag)
	}
}
