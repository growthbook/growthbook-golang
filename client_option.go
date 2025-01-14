package growthbook

import (
	"log/slog"
	"maps"
	"net/http"
	"net/url"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
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
		c.attributes = value.Obj(attributes)
		return nil
	}
}

// SavedGroups are used to target the same group of users across multiple features and experiments
func WithSavedGroups(savedGroups condition.SavedGroups) ClientOption {
	return func(c *Client) error {
		c.data.savedGroups = savedGroups
		return nil
	}
}

// WithUrl sets url of the current page
func WithUrl(rawUrl string) ClientOption {
	return func(c *Client) error {
		url, err := url.Parse(rawUrl)
		if err != nil {
			return err
		}
		c.url = url
		return nil
	}
}

// WithFeatures definitions (usually pulled from an API or cache)
func WithFeatures(features FeatureMap) ClientOption {
	return func(c *Client) error {
		return c.SetFeatures(features)
	}
}

// WithJsonFeatures definitions (usually pulled from an API or cache)
func WithJsonFeatures(featuresJson string) ClientOption {
	return func(c *Client) error {
		return c.SetJSONFeatures(featuresJson)
	}
}

func WithEncryptedJsonFeatures(featuresJson string) ClientOption {
	return func(c *Client) error {
		return c.SetEncryptedJSONFeatures(featuresJson)
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

// WithExtraData sets extra data that will be to callback calls
func WithExtraData(extraData any) ClientOption {
	return func(c *Client) error {
		c.extraData = extraData
		return nil
	}
}

// WithExperiementCallbaback sets experiment callback function
func WithExperimentCallback(cb ExperimentCallback) ClientOption {
	return func(c *Client) error {
		c.experimentCallback = cb
		return nil
	}
}

// WithFeatureUsageCallback sets feature usage callback function
func WithFeatureUsageCallback(cb FeatureUsageCallback) ClientOption {
	return func(c *Client) error {
		c.featureUsageCallback = cb
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

// WithAttributeOverrides creates child client instance with updated top-level attributes
func (c *Client) WithAttributeOverrides(attributes Attributes) (*Client, error) {
	newAttrs := maps.Clone(c.attributes)
	maps.Copy(newAttrs, value.Obj(attributes))
	return c.cloneWith(withValueAttributes(newAttrs))
}

// WithUrl creates child client with updated current page URL
func (c *Client) WithUrl(rawUrl string) (*Client, error) {
	return c.cloneWith(WithUrl(rawUrl))
}

// WithForcedVariations creates child client with updated forced variations
func (c *Client) WithForcedVariations(forcedVariations ForcedVariationsMap) (*Client, error) {
	return c.cloneWith(WithForcedVariations(forcedVariations))
}

// WithExtraData creates child client with extra data that will be sent to a callback
func (c *Client) WithExtraData(extraData any) (*Client, error) {
	return c.cloneWith(WithExtraData(extraData))
}

// WithExperimentCallback creates child client with updated experiment callback function
func (c *Client) WithExperimentCallback(cb ExperimentCallback) (*Client, error) {
	return c.cloneWith(WithExperimentCallback(cb))
}

// WithFeatureUsageCallback creates child client with udpated feature usage callback function
func (c *Client) WithFeatureUsageCallback(cb FeatureUsageCallback) (*Client, error) {
	return c.cloneWith(WithFeatureUsageCallback(cb))
}

func withValueAttributes(value value.ObjValue) ClientOption {
	return func(c *Client) error {
		c.attributes = value
		return nil
	}
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
