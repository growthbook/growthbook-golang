package growthbook

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/r3labs/sse/v2"
)

type repositoryKey string

func RepoRefreshFeatures(gb *GrowthBook, timeout time.Duration,
	skipCache bool, allowStale bool, updateInstance bool) {
	data := fetchFeaturesWithCache(gb, timeout, allowStale, skipCache)
	if updateInstance && data != nil {
		refreshInstance(gb.inner, data)
	}
}

func RepoSubscribe(gb *GrowthBook) { refresh.add(gb) }

func RepoUnsubscribe(gb *GrowthBook) { refresh.remove(gb) }

// -----------------------------------------------------------------------------
//
//  PRIVATE FUNCTIONS START HERE

func fetchFeaturesWithCache(gb *GrowthBook, timeout time.Duration,
	allowStale bool, skipCache bool) *FeatureAPIResponse {
	key := getKey(gb)
	now := time.Now()
	cache.initialize()
	existing := cache.get(key)

	if existing != nil && !skipCache && (allowStale || existing.staleAt.After(now)) {
		if existing.staleAt.Before(now) {
			// Reload features in the backgroud if stale
			go fetchFeatures(gb)
		} else {
			// Otherwise, if we don't need to refresh now, start a
			// background sync.
			refresh.runBackgroundRefresh(gb)
		}
		return existing.data
	} else {
		return fetchFeaturesWithTimeout(gb, timeout)
	}
}

func refreshInstance(inner *growthBookData, data *FeatureAPIResponse) {
	// We are updated values on the inner growthBookData data structures
	// of GrowthBook instances. See the comment on the New function in
	// growthbook.go for an explanation.
	if data.EncryptedFeatures != "" {
		inner.withEncryptedFeatures(data.EncryptedFeatures, "")
	} else {
		features := data.Features
		if features == nil {
			features = inner.features()
		}
		inner.withFeatures(features)
	}
}

// Actually do the HTTP request to get feature data.

func doFetchRequest(gb *GrowthBook) *FeatureAPIResponse {
	apiHost, clientKey := gb.GetAPIInfo()
	key := makeKey(apiHost, clientKey)
	endpoint := apiHost + "/api/features/" + clientKey

	resp, err := http.Get(endpoint)
	if err != nil {
		logErrorf("Error fetching features: HTTP error: endpoint=%s error=%v", endpoint, err)
		return nil
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logErrorf("Error fetching features: reading response body: %v", err)
		return nil
	}

	var apiResponse *FeatureAPIResponse
	err = json.Unmarshal(body, &apiResponse)
	if err != nil || apiResponse == nil {
		logErrorf("Error fetching features: parsing response: %v", err)
		return nil
	}

	// Record whether this endpoint supports SSE updates.
	sse, ok := resp.Header["X-Sse-Support"]
	refresh.sseSupported(key, ok && sse[0] == "enabled")

	return apiResponse
}

// Mutex-protected map holding channels to concurrent requests for
// features for the same repository key. Only one real HTTP request is
// in flight at any time for a given repository key.

var outstandingRequestMutex sync.Mutex
var outstandingRequest map[repositoryKey][]chan *FeatureAPIResponse

// We need to be able to clear the outstanding requests when the cache
// is cleared.

func clearOutstandingRequests() {
	outstandingRequestMutex.Lock()
	defer outstandingRequestMutex.Unlock()
	outstandingRequest = make(map[repositoryKey][]chan *FeatureAPIResponse)
}

// Retrieve features from the API, ensuring that only one request for
// any given repository key is in flight at any time.

func fetchFeatures(gb *GrowthBook) *FeatureAPIResponse {
	apiHost, clientKey := gb.GetAPIInfo()
	key := makeKey(apiHost, clientKey)

	// The first request for a given repository key will put a nil
	// channel value into the relevant slot of the outstandingRequest
	// map. Subsequent requests for the same repository key that come in
	// while the first request is being processed will create a channel
	// to receive the results from the in flight request.
	outstandingRequestMutex.Lock()
	if outstandingRequest == nil {
		outstandingRequest = make(map[repositoryKey][]chan *FeatureAPIResponse)
	}
	chans := outstandingRequest[key]
	myChan := make(chan *FeatureAPIResponse)
	first := false
	if chans == nil {
		first = true
		outstandingRequest[key] = []chan *FeatureAPIResponse{}
	}
	outstandingRequest[key] = append(outstandingRequest[key], myChan)
	outstandingRequestMutex.Unlock()

	// Either:
	var apiResponse *FeatureAPIResponse
	if first {
		// We were the first request to come in, so perform the API
		// request, and...
		apiResponse = doFetchRequest(gb)

		// ...retrieve a list of channels to other goroutines requesting
		// features for the same repository key, clearing the outstanding
		// requests slot for this repository key...
		outstandingRequestMutex.Lock()
		chans := outstandingRequest[key]
		delete(outstandingRequest, key)
		outstandingRequestMutex.Unlock()

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
			refresh.runBackgroundRefresh(gb)
		}
	} else {
		// We were a later request, so just wait for the result from the
		// goroutine performing the request on our channel.
		apiResponse = <-myChan
	}

	// If something went wrong, we return an empty response, rather than
	// nil.
	if apiResponse == nil {
		apiResponse = &FeatureAPIResponse{}
	}
	return apiResponse
}

func fetchFeaturesWithTimeout(gb *GrowthBook, timeout time.Duration) *FeatureAPIResponse {
	if timeout == 0 {
		return fetchFeatures(gb)
	}
	ch := make(chan *FeatureAPIResponse, 1)
	timer := time.NewTimer(timeout)
	go func() {
		ch <- fetchFeatures(gb)
	}()
	select {
	case result := <-ch:
		return result
	case <-timer.C:
		return nil
	}
}

func onNewFeatureData(key repositoryKey, data *FeatureAPIResponse) {
	// If contents haven't changed, ignore the update, extend the stale TTL
	version := data.DateUpdated
	now := time.Now()
	staleAt := now.Add(cacheStaleTTL)
	existing := cache.get(key)
	if existing != nil && version != "" && existing.version == version {
		existing.staleAt = staleAt
		return
	}

	// Update in-memory cache
	cache.set(key, &cacheEntry{data, version, staleAt})

	// Update features for all subscribed GrowthBook instances.
	for _, inner := range refresh.instances(key) {
		refreshInstance(inner, data)
	}
}

// -----------------------------------------------------------------------------
//
//  AUTO-REFRESH PROCESSING

// We store *only* the inner data structure of GrowthBook instances
// here, so that the finalizer added to the main (outer) GrowthBook
// instances will run, triggering an unsubscribe, allowing us to
// remove the inner data structure here.
type gbDataSet map[*growthBookData]bool

type refreshData struct {
	sync.RWMutex

	// Repository keys where SSE is supported.
	// TODO: THINK OF A BETTER WAY TO MANAGE THIS?
	sse map[repositoryKey]bool

	// Channels to shut down SSE refresh goroutines.
	shutdown map[repositoryKey]chan struct{}

	// Subscribed instances.
	subscribed map[repositoryKey]gbDataSet
}

func makeRefreshData() *refreshData {
	return &refreshData{
		sse:        make(map[repositoryKey]bool),
		shutdown:   make(map[repositoryKey]chan struct{}),
		subscribed: make(map[repositoryKey]gbDataSet),
	}
}

var refresh *refreshData = makeRefreshData()

func clearAutoRefresh() {
	refresh.stop()
	refresh = makeRefreshData()
}

func (r *refreshData) instances(key repositoryKey) []*growthBookData {
	r.RLock()
	defer r.RUnlock()

	m := r.subscribed[key]
	if m == nil {
		return []*growthBookData{}
	}
	result := make([]*growthBookData, len(m))
	i := 0
	for k := range m {
		result[i] = k
		i++
	}
	return result
}

func (r *refreshData) stop() {
	r.Lock()
	defer r.Unlock()

	for _, ch := range r.shutdown {
		ch <- struct{}{}
	}
}

func (r *refreshData) add(gb *GrowthBook) {
	// Add a subscription.
	// TODO: START THE AUTO-REFRESH GOROUTINE IF NEEDED? NOT SURE.

	r.Lock()
	defer r.Unlock()

	key := getKey(gb)
	subs := r.subscribed[key]
	if subs == nil {
		subs = make(gbDataSet)
	}
	subs[gb.inner] = true
	r.subscribed[key] = subs
}

func (r *refreshData) remove(gb *GrowthBook) {
	// Remove a subscription. Also closes down the auto-refresh
	// goroutine if there is one and this is the last subscriber.

	r.Lock()
	defer r.Unlock()

	key := getKey(gb)
	subs := r.subscribed[key]
	if subs != nil {
		delete(subs, gb.inner)
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

func (r *refreshData) sseSupported(key repositoryKey, supported bool) {
	r.Lock()
	defer r.Unlock()
	r.sse[key] = supported
}

func (r *refreshData) runBackgroundRefresh(gb *GrowthBook) {
	r.Lock()
	defer r.Unlock()

	key := getKey(gb)

	// Conditions required to proceed here:
	//  - Background sync must be enabled.
	//  - The repository must support SSE.
	//  - Background sync must not already be running for the repository.
	if !cacheBackgroundSync || !r.sse[key] || r.shutdown[key] != nil {
		return
	}

	ch := make(chan struct{})
	refresh.shutdown[key] = ch
	go refreshFromSSE(gb, ch)
}

func refreshFromSSE(gb *GrowthBook, shutdown chan struct{}) {
	apiHost, clientKey := gb.GetAPIInfo()
	key := makeKey(apiHost, clientKey)

	var client *sse.Client
	ch := make(chan *sse.Event)
	reconnect := make(chan struct{}, 1)
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
			client.SubscribeChan("features", ch)

		case msg := <-ch:
			var data FeatureAPIResponse
			err := json.Unmarshal(msg.Data, &data)

			if err != nil && client != nil {
				logErrorf("SSE error: %s", key)
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
			onNewFeatureData(key, &data)
		}
	}
}

// -----------------------------------------------------------------------------
//
//  CACHING
//

var cacheBackgroundSync bool = true
var cacheStaleTTL time.Duration = 60 * time.Second

func ConfigureCacheBackgroundSync(bgSync bool) {
	cacheBackgroundSync = bgSync
	if !bgSync {
		clearAutoRefresh()
	}
}

func ConfigureCacheStaleTTL(ttl time.Duration) {
	cacheStaleTTL = ttl
}

type cacheEntry struct {
	data    *FeatureAPIResponse
	version string
	staleAt time.Time
}

type repoCache struct {
	sync.RWMutex
	data map[repositoryKey]*cacheEntry
}

var cache repoCache

func (c *repoCache) initialize() {
}

func (c *repoCache) clear() {
	c.Lock()
	defer c.Unlock()

	// Clear cache, auto-refresh info and outstanding requests.
	c.data = make(map[repositoryKey]*cacheEntry)
	clearAutoRefresh()
	clearOutstandingRequests()
}

func (c *repoCache) get(key repositoryKey) *cacheEntry {
	c.RLock()
	defer c.RUnlock()
	return c.data[key]
}

func (c *repoCache) set(key repositoryKey, entry *cacheEntry) {
	c.Lock()
	defer c.Unlock()
	c.data[key] = entry
}

// -----------------------------------------------------------------------------
//
//  UTILITIES

func getKey(gb *GrowthBook) repositoryKey {
	apiHost, clientKey := gb.GetAPIInfo()
	return repositoryKey(apiHost + "||" + clientKey)
}

func makeKey(apiHost string, clientKey string) repositoryKey {
	return repositoryKey(apiHost + "||" + clientKey)
}
