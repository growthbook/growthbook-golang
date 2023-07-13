package growthbook

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/r3labs/sse/v2"
)

// Alias for names of repositories. Used as key type in various maps.
// The key for a given repository is of the form
// "<apiHost>||<clientKey>".

type RepositoryKey string

// Interface for feature caching.

type Cache interface {
	Initialize()
	Clear()
	Get(key RepositoryKey) *CacheEntry
	Set(key RepositoryKey, entry *CacheEntry)
}

// Cache entry type for feature cache.

type CacheEntry struct {
	Data    *FeatureAPIResponse `json:"data"`
	Version time.Time           `json:"version"`
	StaleAt time.Time           `json:"stale_at"`
}

// Set feature cache. Passing nil uses the default in-memory cache.

func ConfigureCache(c Cache) {
	if c == nil {
		c = &repoCache{}
	}
	cache.Clear()
	cache = c
}

// ConfigureCacheBackgroundSync enables or disables background cache
// synchronization.

func ConfigureCacheBackgroundSync(bgSync bool) {
	cacheBackgroundSync = bgSync
	if !bgSync {
		clearAutoRefresh()
	}
}

// -----------------------------------------------------------------------------
//
//  PRIVATE FUNCTIONS START HERE

// repoRefreshFeatures fetches features from the GrowthBook API and
// updates the calling GrowthBook instances as required.

func repoRefreshFeatures(ctx context.Context, c *Client, timeout time.Duration,
	skipCache bool, allowStale bool, updateInstance bool) error {
	data, err := fetchFeaturesWithCache(ctx, c, timeout, allowStale, skipCache)
	if updateInstance && data != nil {
		refreshInstance(c.features, data)
	}
	return err
}

func repoLatestUpdate(c *Client) *time.Time {
	key := getKey(c)
	existing := cache.Get(key)
	if existing == nil {
		return nil
	}
	return &existing.Version
}

// RepoSubscribe adds a subscription for automatic feature updates for
// a GrowthBook client instance. Feature values for the instance are
// updated transparently when new values are retrieved from the API
// (either by explicit requests or via SSE updates).

func repoSubscribe(c *Client) { refresh.addSubscription(c) }

// RepoUnsubscribe removes a subscription for automatic feature
// updates for a GrowthBook client instance.

func repoUnsubscribe(c *Client) { refresh.removeSubscription(c) }

// ConfigureCacheStaleTTL sets the time-to-live duration for feature
// cache entries.

func ConfigureCacheStaleTTL(ttl time.Duration) {
	if ttl == 0 {
		ttl = 60 * time.Second
	}
	cacheStaleTTL = ttl
}

// Top-level feature fetching function. Responsible for caching,
// starting background refresh goroutines, and timeout management for
// API request, which is handed off to fetchFeatures.

func fetchFeaturesWithCache(ctx context.Context, c *Client, timeout time.Duration,
	allowStale bool, skipCache bool) (*FeatureAPIResponse, error) {
	key := getKey(c)
	now := time.Now()
	cache.Initialize()
	existing := cache.Get(key)

	if existing != nil && !skipCache && (allowStale || existing.StaleAt.After(now)) {
		if existing.StaleAt.Before(now) {
			// Reload features in the backgroud if stale
			go fetchFeatures(ctx, c)
		} else {
			// Otherwise, if we don't need to refresh now, start a
			// background sync.
			refresh.runBackgroundRefresh(ctx, c)
		}
		return existing.Data, nil
	} else {
		// Perform API request with timeout.
		if timeout == 0 {
			return fetchFeatures(ctx, c)
		}
		type response struct {
			result *FeatureAPIResponse
			err    error
		}
		ch := make(chan *response, 1)
		timer := time.NewTimer(timeout)
		go func() {
			result, err := fetchFeatures(ctx, c)
			ch <- &response{result, err}
		}()
		select {
		case result := <-ch:
			return result.result, result.err
		case <-timer.C:
			return nil, nil
		}
	}
}

// Mutex-protected map holding channels to concurrent requests for
// features for the same repository key. Only one real HTTP request is
// in flight at any time for a given repository key.

var outstandingRequestMutex sync.Mutex
var outstandingRequest map[RepositoryKey][]chan *FeatureAPIResponse

// We need to be able to clear the outstanding requests when the cache
// is cleared.

func clearOutstandingRequests() {
	outstandingRequestMutex.Lock()
	defer outstandingRequestMutex.Unlock()

	outstandingRequest = make(map[RepositoryKey][]chan *FeatureAPIResponse)
}

// Retrieve features from the API, ensuring that only one request for
// any given repository key is in flight at any time.

func fetchFeatures(ctx context.Context, c *Client) (*FeatureAPIResponse, error) {
	apiHost, clientKey := c.GetAPIInfo()
	key := makeKey(apiHost, clientKey)

	// Get outstanding request channel, and flag to indicate whether
	// this is the first channel created for this key.
	myChan, first := addRequestChan(key)

	// Either:
	var apiResponse *FeatureAPIResponse
	var err error
	if first {
		// We were the first request to come in, so perform the API
		// request, and...
		apiResponse, err = doFetchRequest(ctx, c)

		// ...retrieve a list of channels to other goroutines requesting
		// features for the same repository key, clearing the outstanding
		// requests slot for this repository key...
		chans := removeRequestChan(key)

		// ...then send the API response to all the waiting goroutines. We
		// check that our channel is still in the list, in case the cache
		// and the outstanding requests information has been cleared while
		// we were making the request.
		selfFound := false
		for _, ch := range chans {
			if ch != myChan {
				ch <- apiResponse
			} else {
				// Don't send to ourselves, but record that our channel is
				// still in the list.
				selfFound = true
			}
		}

		// Finally call the new feature data callback (from a single
		// goroutine), assuming that the outstanding requests list hasn't
		// been cleared in the meantime.
		if apiResponse != nil && selfFound {
			onNewFeatureData(key, apiResponse)
			refresh.runBackgroundRefresh(ctx, c)
		}
	} else {
		// We were a later request, so just wait for the result from the
		// goroutine performing the request on our channel.
		apiResponse = <-myChan
	}

	// If something went wrong, we return an empty response, rather than
	// nil.
	if err != nil || apiResponse == nil {
		apiResponse = &FeatureAPIResponse{}
	}
	return apiResponse, err
}

// The first request for a given repository key will put a nil channel
// value into the relevant slot of the outstandingRequest map.
// Subsequent requests for the same repository key that come in while
// the first request is being processed will create a channel to
// receive the results from the in flight request.

func addRequestChan(key RepositoryKey) (chan *FeatureAPIResponse, bool) {
	outstandingRequestMutex.Lock()
	defer outstandingRequestMutex.Unlock()

	if outstandingRequest == nil {
		outstandingRequest = make(map[RepositoryKey][]chan *FeatureAPIResponse)
	}
	chans := outstandingRequest[key]
	myChan := make(chan *FeatureAPIResponse)
	first := false
	if chans == nil {
		first = true
		outstandingRequest[key] = []chan *FeatureAPIResponse{}
	}
	outstandingRequest[key] = append(outstandingRequest[key], myChan)

	return myChan, first
}

// Remove the request channel for a given key.

func removeRequestChan(key RepositoryKey) []chan *FeatureAPIResponse {
	outstandingRequestMutex.Lock()
	defer outstandingRequestMutex.Unlock()

	chans := outstandingRequest[key]
	delete(outstandingRequest, key)
	return chans
}

// Actually do the HTTP request to get feature data.

func doFetchRequest(ctx context.Context, c *Client) (*FeatureAPIResponse, error) {
	apiHost, clientKey := c.GetAPIInfo()
	key := makeKey(apiHost, clientKey)
	endpoint := apiHost + "/api/features/" + clientKey

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		err = fmt.Errorf("Error fetching features: can't create request: [%w]", err)
		return nil, err
	}
	resp, err := c.opt.HTTPClient.Do(req)
	if err != nil {
		err = fmt.Errorf("Error fetching features (endpoint=%s): HTTP error [%w]",
			endpoint, err)
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil || len(body) == 0 {
			body = []byte("<none>")
		}
		err = fmt.Errorf("Error fetching features (endpoint=%s): HTTP error: status=%d body=%s",
			endpoint, resp.StatusCode, string(body))
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("Error fetching features: reading response body: [%w]", err)
		return nil, err
	}

	apiResponse := FeatureAPIResponse{}
	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		err = fmt.Errorf("Error fetching features: parsing response: [%w]", err)
		return nil, err
	}

	// Record whether this endpoint supports SSE updates.
	sse, ok := resp.Header["X-Sse-Support"]
	refresh.sseSupported(key, ok && sse[0] == "enabled")

	return &apiResponse, nil
}

// Update values on the inner featureData data structures of
// GrowthBook client instances. See the comment on the NewClient
// function in client.go for an explanation.

func refreshInstance(feats *featureData, data *FeatureAPIResponse) {
	if data.EncryptedFeatures != "" {
		err := feats.withEncryptedFeatures(data.EncryptedFeatures, "")
		if err != nil {
			logError("failed to decrypt encrypted features")
		}
	} else {
		features := data.Features
		if features == nil {
			features = feats.getFeatures()
		}
		feats.withFeatures(features)
	}
}

// Callback to process feature updates from API, via both explicit
// requests and background processing.

func onNewFeatureData(key RepositoryKey, data *FeatureAPIResponse) {
	// If contents haven't changed, ignore the update and extend the
	// stale TTL.
	version := data.DateUpdated
	now := time.Now()
	staleAt := now.Add(cacheStaleTTL)
	existing := cache.Get(key)
	if existing != nil && existing.Version == version {
		existing.StaleAt = staleAt
		return
	}

	// Update in-memory cache.
	cache.Set(key, &CacheEntry{data, version, staleAt})

	// Update features for all subscribed GrowthBook client instances.
	for _, feats := range refresh.instances(key) {
		refreshInstance(feats, data)
	}
}

// -----------------------------------------------------------------------------
//
//  AUTO-REFRESH PROCESSING

// We store *only* the inner data structure of GrowthBook client
// instances here, so that the finalizer added to the main (outer)
// GrowthBook client instances will run, triggering an unsubscribe,
// allowing us to remove the inner data structure here.
type gbDataSet map[*featureData]bool

type refreshData struct {
	sync.RWMutex

	// Repository keys where SSE is supported.
	sse map[RepositoryKey]bool

	// Channels to shut down SSE refresh goroutines.
	shutdown map[RepositoryKey]chan struct{}

	// Channels to force reconnect of SSE refresh goroutines.
	reconnect map[RepositoryKey]chan struct{}

	// Subscribed instances.
	subscribed map[RepositoryKey]gbDataSet
}

func makeRefreshData() *refreshData {
	return &refreshData{
		sse:        make(map[RepositoryKey]bool),
		shutdown:   make(map[RepositoryKey]chan struct{}),
		reconnect:  make(map[RepositoryKey]chan struct{}),
		subscribed: make(map[RepositoryKey]gbDataSet),
	}
}

var refresh *refreshData = makeRefreshData()

func clearAutoRefresh() {
	refresh.stop()
	refresh = makeRefreshData()
}

func reconnectAutoRefresh() {
	refresh.forceReconnect()
}

// Safely get list of GrowthBook client instance inner data structures
// for a repository key.

func (r *refreshData) instances(key RepositoryKey) []*featureData {
	r.RLock()
	defer r.RUnlock()

	m := r.subscribed[key]
	if m == nil {
		return []*featureData{}
	}
	result := make([]*featureData, len(m))
	i := 0
	for k := range m {
		result[i] = k
		i++
	}
	return result
}

// Shut down data refresh machinery.

func (r *refreshData) stop() {
	r.Lock()
	defer r.Unlock()

	for _, ch := range r.shutdown {
		ch <- struct{}{}
	}
}

// Force reconnect of all SSE data refresh goroutines.

func (r *refreshData) forceReconnect() {
	r.Lock()
	defer r.Unlock()

	for _, ch := range r.reconnect {
		ch <- struct{}{}
	}
}

// Add a subscription.

func (r *refreshData) addSubscription(c *Client) {
	r.Lock()
	defer r.Unlock()

	key := getKey(c)
	subs := r.subscribed[key]
	if subs == nil {
		subs = make(gbDataSet)
	}
	subs[c.features] = true
	r.subscribed[key] = subs
}

// Remove a subscription. Also closes down the auto-refresh goroutine
// if there is one and this is the last subscriber.

func (r *refreshData) removeSubscription(c *Client) {
	r.Lock()
	defer r.Unlock()

	key := getKey(c)
	subs := r.subscribed[key]
	if subs != nil {
		delete(subs, c.features)
		if len(subs) == 0 {
			subs = nil
		}
	}
	r.subscribed[key] = subs

	if subs == nil {
		ch := r.shutdown[key]
		if ch != nil {
			ch <- struct{}{}
			delete(r.shutdown, key)
		}
	}
}

func (r *refreshData) sseSupported(key RepositoryKey, supported bool) {
	r.Lock()
	defer r.Unlock()

	r.sse[key] = supported
}

func (r *refreshData) runBackgroundRefresh(ctx context.Context, c *Client) {
	r.Lock()
	defer r.Unlock()

	key := getKey(c)

	// Conditions required to proceed here:
	//  - Background sync must be enabled.
	//  - The repository must support SSE.
	//  - Background sync must not already be running for the repository.
	if !cacheBackgroundSync || !r.sse[key] || r.shutdown[key] != nil {
		return
	}

	shutdown := make(chan struct{})
	refresh.shutdown[key] = shutdown
	reconnect := make(chan struct{}, 1)
	refresh.reconnect[key] = reconnect
	go refreshFromSSE(ctx, c, shutdown, reconnect)
}

func refreshFromSSE(ctx context.Context, c *Client,
	shutdown chan struct{}, reconnect chan struct{}) {
	apiHost, clientKey := c.GetAPIInfo()
	key := makeKey(apiHost, clientKey)

	var client *sse.Client
	ch := make(chan *sse.Event)
	reconnect <- struct{}{}
	var errors int

	for {
		select {
		case <-shutdown:
			return

		case <-reconnect:
			logInfof("Connecting to SSE stream: %s", key)
			errors = 0
			client := sse.NewClient(apiHost + "/sub/" + clientKey)
			client.OnDisconnect(func(c *sse.Client) {
				logErrorf("SSE event stream disconnected: %s", key)
				reconnect <- struct{}{}
			})
			client.SubscribeChanWithContext(ctx, "features", ch)

		case msg := <-ch:
			if len(msg.Data) == 0 {
				break
			}
			var data FeatureAPIResponse
			err := json.Unmarshal(msg.Data, &data)

			if err != nil {
				logErrorf("SSE error (%s): %v", key, err)
			}
			if err != nil && client != nil {
				errors++
				if errors > 3 {
					logErrorf("Multiple SSE errors: disconnecting stream: %s", key)
					client.Unsubscribe(ch)
					client = nil

					// Exponential backoff after 4 errors, with jitter.
					msDelay := math.Pow(3, float64(errors-3)) * (1000 + rand.Float64()*1000)
					delay := time.Duration(msDelay) * time.Millisecond

					// 5 minutes max.
					if delay > 5*time.Minute {
						delay = 5 * time.Minute
					}
					logWarnf("Waiting to reconnect SSE stream: %s (delaying %s)", key, delay)
					time.Sleep(delay)
					reconnect <- struct{}{}
				}
				continue
			}
			logInfo("New feature data from SSE stream")
			onNewFeatureData(key, &data)
		}
	}
}

// -----------------------------------------------------------------------------
//
//  CACHING
//

// Cache control parameters.

var cacheBackgroundSync bool = true
var cacheStaleTTL time.Duration = 60 * time.Second

// Default in-memory cache.

type repoCache struct {
	sync.RWMutex
	data map[RepositoryKey]*CacheEntry
}

var cache Cache = &repoCache{data: map[RepositoryKey]*CacheEntry{}}

func (c *repoCache) Initialize() {}

func (c *repoCache) Clear() {
	c.Lock()
	defer c.Unlock()

	// Clear cache, auto-refresh info and outstanding requests.
	c.data = make(map[RepositoryKey]*CacheEntry)
	clearAutoRefresh()
	clearOutstandingRequests()
}

func (c *repoCache) Get(key RepositoryKey) *CacheEntry {
	c.RLock()
	defer c.RUnlock()

	return c.data[key]
}

func (c *repoCache) Set(key RepositoryKey, entry *CacheEntry) {
	c.Lock()
	defer c.Unlock()

	c.data[key] = entry
}

// -----------------------------------------------------------------------------
//
//  REPOSITORY KEY UTILITIES

func getKey(c *Client) RepositoryKey {
	apiHost, clientKey := c.GetAPIInfo()
	return RepositoryKey(apiHost + "||" + clientKey)
}

func makeKey(apiHost string, clientKey string) RepositoryKey {
	return RepositoryKey(apiHost + "||" + clientKey)
}
