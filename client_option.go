package growthbook

import (
	"log/slog"
	"net/http"
)

type ClientOption func(*Client) error

// WithEnabled switch to globally disable all experiments. Default true.
func WithEnabled(enabled bool) ClientOption {
	return func(c *Client) error {
		c.enabled = enabled
		return nil
	}
}

// WithApiHost sets the  GrowthBook API Host.
func WithApiHost(apiHost string) ClientOption {
	return func(c *Client) error {
		c.data.apiHost = apiHost
		return nil
	}
}

// WithClientKey used to fetch features from the GrowthBook API.
func WithClientKey(clientKey string) ClientOption {
	return func(c *Client) error {
		c.data.clientKey = clientKey
		return nil
	}
}

// WithDecryptionKey used to decrypt encrypted features from the API. Optional
func WithDecryptionKey(decryptionKey string) ClientOption {
	return func(c *Client) error {
		c.data.decryptionKey = decryptionKey
		return nil
	}
}

// Attributes are used to assign variations
func WithAttributes(attributes Attributes) ClientOption {
	return func(c *Client) error {
		c.attributes = attributes
		return nil
	}
}

// WithUrl sets url of the current page
func WithUrl(url string) ClientOption {
	return func(c *Client) error {
		c.url = url
		return nil
	}
}

// WithFeatures definitions (usually pulled from an API or cache)
func WithFeatures(features FeatureMap) ClientOption {
	return func(c *Client) error {
		c.data.features = features
		return nil
	}
}

// WithForcedVariations force specific experiments to always assign a specific variation (used for QA)
func WithForcedVariations(forcedVariations ForcedVariationsMap) ClientOption {
	return func(c *Client) error {
		c.forcedVariations = forcedVariations
		return nil
	}
}

// WithQaMode if true, random assignment is disabled and only explicitly forced variations are used.
func WithQaMode(qaMode bool) ClientOption {
	return func(c *Client) error {
		c.qaMode = qaMode
		return nil
	}
}

// WithTrackingCallback a function that takes experiment and result as arguments.
func WithTrackingCallback(trackingCallback TrackingCallback) ClientOption {
	return func(c *Client) error {
		c.trackingCallback = trackingCallback
		return nil
	}
}

// WithHttpClient sets http client for GrowthBook API calls
func WithHttpClient(httpClient *http.Client) ClientOption {
	return func(c *Client) error {
		c.data.httpClient = httpClient
		return nil
	}
}

// WithLogger sets logger for GrowthBook client
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) error {
		c.logger = logger
		return nil
	}
}

// Child client instance options

// WithEnabled creates child client instance with updated enabled switch
func (c *Client) WithEnabled(enabled bool) (*Client, error) {
	return c.cloneWith(WithEnabled(enabled))
}

// WithQaMode creates child client instance with updated qaMode switch
func (c *Client) WithQaMode(qaMode bool) (*Client, error) {
	return c.cloneWith(WithQaMode(qaMode))
}

// WithLogger creates child client instance that uses provided logger
func (c *Client) WithLogger(logger *slog.Logger) (*Client, error) {
	return c.cloneWith(WithLogger(logger))
}

// WithAttributes creates child client instance that uses provided attributes for evaluation
func (c *Client) WithAttributes(attributes Attributes) (*Client, error) {
	return c.cloneWith(WithAttributes(attributes))
}

func (c *Client) cloneWith(opts ...ClientOption) (*Client, error) {
	clone := c.clone()
	for _, opt := range opts {
		err := opt(clone)
		if err != nil {
			return nil, err
		}
	}
	return clone, nil
}
