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
	Status            int                   `json:"status"`
	Features          FeatureMap            `json:"features"`
	DateUpdated       time.Time             `json:"dateUpdated"`
	SavedGroups       condition.SavedGroups `json:"savedGroups"`
	EncryptedFeatures string                `json:"encryptedFeatures"`
	SseSupport        bool
	Etag              string
}

const userAgent = "Growhthbook Go SDK client"

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

	apiResp.Status = resp.StatusCode
	apiResp.Etag = resp.Header.Get("etag")
	apiResp.SseSupport = resp.Header.Get("x-sse-support") == "enabled"

	if resp.StatusCode == 304 {
		return &apiResp, nil
	}

	if resp.StatusCode != 200 {
		return &apiResp, fmt.Errorf("Error loading features, code: %d", resp.StatusCode)
	}

	defer resp.Body.Close()
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
