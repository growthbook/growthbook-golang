package growthbook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
			fetchFeatures(gb)
		} else {
			// Otherwise, if we don't need to refresh now, start a background sync
			startAutoRefresh(gb)
		}
		return existing.data
	} else {
		// Handle timeout here.
		return fetchFeatures(gb)
	}
}

func refreshInstance(inner *growthBookData, data *FeatureAPIResponse) {
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

func fetchFeatures(gb *GrowthBook) *FeatureAPIResponse {
	apiHost, clientKey := gb.GetAPIInfo()
	endpoint := apiHost + "/api/features/" + clientKey

	resp, err := http.Get(endpoint)
	if err != nil {
		logErrorf("HTTP GET error: endpoint=%s error=%v", endpoint, err)
		return nil
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logErrorf("Error reading HTTP GET response body: %v", err)
		return nil
	}

	apiResponse := FeatureAPIResponse{}
	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		logErrorf("Error parsing HTTP GET response body: %v", err)
		return nil
	}

	sse, ok := resp.Header["X-Sse-Support"]
	key := makeKey(apiHost, clientKey)
	refresh.sseSupported(key, ok && sse[0] == "enabled")

	onNewFeatureData(key, &apiResponse)
	return &apiResponse
}

func onNewFeatureData(key repositoryKey, data *FeatureAPIResponse) {
	// If contents haven't changed, ignore the update, extend the stale TTL
	version := data.DateUpdated
	staleAt := time.Now().Add(cacheStaleTTL)
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
	// Add a subscription. Starts the auto-refresh goroutine if needed.

	r.Lock()
	defer r.Unlock()

	key := getKey(gb)
	subs := r.subscribed[key]
	if subs == nil {
		subs = make(gbDataSet)
	}
	subs[gb.inner] = true
	r.subscribed[key] = subs

	// ch := r.shutdown[key]
	// if ch == nil {
	// 	// TODO: ONLY DO THIS IF SSE SUPPORT IS NEEDED!
	// 	r.shutdown[key] = make(chan struct{})
	// 	// START SSE REFRESH GOROUTINE.
	// }
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

func (r *refreshData) runSSERefresh(gb *GrowthBook) {
	r.Lock()
	defer r.Unlock()

	key := getKey(gb)

	if !r.sse[key] {
		// Doesn't support SSE.
		return
	}
	if r.shutdown[key] != nil {
		// Already set up.
		return
	}

	ch := make(chan struct{})
	refresh.shutdown[key] = ch
	go refreshFromSSE(gb, ch)
}

// ISSUES:
//
// 1. Ownership/GC of GrowthBook instances
//
// We keep hold of pointers to GrowthBook instances within the refresh
// goroutine. These only get removed when an explicit unsubscribe
// commandis sent to the refresh goroutine.
//
// What happens when a GrowthBook instance goes out of scope in the
// place where it was created? How does it get removed from the array
// inside the refresh goroutine, so allowing it to be GCed?
//
// Do we need an explicit Destroy method on the GrowthBook type?
//
// Options:
//
//  - Somehow use a finalizer. I think this means the GrowthBook type
//    has to be a wrapper around a GrowthBookData type. You could put
//    a finalizer on the outer type, and use that finalizer to remove
//    references to the inner type. You would store references only to
//    the inner type in data structures used by autonomous goroutines.
//
// 2. Termination of refresh goroutines
//
// Should we just let the goroutine exit when the subscribed list
// becomes empty? We also need to remove the input channel from the
// refreshChans map when that happens, since there won't be anyone
// listening on that channel any more.
//
// Options:
//
//  - Use a single refresh goroutine with a mutex to handle starting
//    and stopping it?

func refreshFromSSE(gb *GrowthBook, shutdown chan struct{}) {
	apiHost, clientKey := gb.GetAPIInfo()
	key := makeKey(apiHost, clientKey)
	client := sse.NewClient(apiHost + "/sub/" + clientKey)
	ch := make(chan *sse.Event)
	client.SubscribeChan("features", ch)

	done := false
	for !done {
		select {
		case <-shutdown:
			done = true

		case msg := <-ch:
			// TODO: BETTER ERROR HANDLING!
			var data FeatureAPIResponse
			err := json.Unmarshal(msg.Data, &data)
			if err != nil {
				logError("Couldn't decode SSE message")
				fmt.Println(string(msg.Data))
				continue
			}
			onNewFeatureData(key, &data)
		}
	}
}

// Watch a feature endpoint for changes
// Will prefer SSE if enabled, otherwise fall back to cron
func startAutoRefresh(gb *GrowthBook) {
	if cacheBackgroundSync {
		refresh.runSSERefresh(gb)
	}
}

//     channel := ScopedChannel = {
//       src: null,
//       cb: (event: MessageEvent<string>) => {
//         try {
//           const json: FeatureApiResponse = JSON.parse(event.data);
//           onNewFeatureData(key, json);
//           // Reset error count on success
//           channel.errors = 0;
//         } catch (e) {
//           process.env.NODE_ENV !== "production" &&
//             instance.log("SSE Error", {
//               apiHost,
//               clientKey,
//               error: e ? (e as Error).message : null,
//             });
//           onSSEError(channel, apiHost, clientKey);
//         }
//       },
//       errors: 0,
//     };
//     streams.set(key, channel);
//     enableChannel(channel, apiHost, clientKey);
//   }
// }

// -----------------------------------------------------------------------------
//
//  CACHING
//

var cacheBackgroundSync bool = true
var cacheStaleTTL time.Duration = 60 * time.Second

func ConfigureCacheBackgroundSync(bgSync bool) {
	cacheBackgroundSync = bgSync
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

	// Clear cache.
	c.data = make(map[repositoryKey]*cacheEntry)

	// Clear auto-refresh info.
	refresh.stop()
	refresh = makeRefreshData()
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
