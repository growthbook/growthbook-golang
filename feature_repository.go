package growthbook

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"golang.org/x/exp/slices"
)

type repositoryKey string

// Repository keys where SSE is supported.
// TODO: THINK OF A BETTER WAY TO MANAGE THIS?
var sseSupported map[repositoryKey]bool

// Channels to SSE refresh goroutines.
var refreshChans map[repositoryKey]chan cmd

func RefreshFeatures(gb *GrowthBook, timeout time.Duration,
	skipCache bool, allowStale bool, updateInstance bool) {
	data := fetchFeaturesWithCache(gb, timeout, allowStale, skipCache)
	if updateInstance && data != nil {
		refreshInstance(gb, data)
	}
}

func SubscribeToFeatures(gb *GrowthBook) {}

func UnsubscribeFromFeatures(gb *GrowthBook) {}

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

func refreshInstance(gb *GrowthBook, data *FeatureAPIResponse) {
	if data.EncryptedFeatures != "" {
		gb.WithEncryptedFeatures(data.EncryptedFeatures, "")
	} else {
		features := data.Features
		if features == nil {
			features = gb.Features()
		}
		gb.WithFeatures(features)
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
	if ok && sse[0] == "enabled" {
		sseSupported[key] = true
	} else {
		delete(sseSupported, key)
	}

	onNewFeatureData(key, &apiResponse)
	return &apiResponse
}

func getKey(gb *GrowthBook) repositoryKey {
	apiHost, clientKey := gb.GetAPIInfo()
	return repositoryKey(apiHost + "||" + clientKey)
}

func makeKey(apiHost string, clientKey string) repositoryKey {
	return repositoryKey(apiHost + "||" + clientKey)
}

type cmdType int

const (
	shutdownCmd cmdType = iota
	subCmd      cmdType = iota
	unsubCmd    cmdType = iota
	refreshCmd  cmdType = iota
)

type cmd struct {
	t    cmdType
	gb   *GrowthBook
	data *FeatureAPIResponse
}

func refreshFromSSE(key repositoryKey, in chan cmd) {
	subscribed := []*GrowthBook{}

	// TODO: SUBSCRIBE TO SSE HERE.
	//sseChan := make(chan *Event)
	//sseChan := make(chan *int)

	for {
		select {
		case cmd := <-in:
			switch cmd.t {
			case shutdownCmd:
				break
			case subCmd:
				index := slices.Index(subscribed, cmd.gb)
				if index == -1 {
					subscribed = append(subscribed, cmd.gb)
				}
			case unsubCmd:
				index := slices.Index(subscribed, cmd.gb)
				if index != -1 {
					subscribed = slices.Delete(subscribed, index, index+1)
					subscribed[len(subscribed)] = nil
				}
			}
			//case msg := <-sseChan:
			// TODO: EXTRACT FeatureAPIResponse FROM SSE DATA

			// onNewFeatureData(key, msg)
		}
	}
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

	// Update features for all subscribed GrowthBook instances
	ch, ok := refreshChans[key]
	if ok {
		ch <- cmd{t: refreshCmd, data: data}
	}
}

// Watch a feature endpoint for changes
// Will prefer SSE if enabled, otherwise fall back to cron
func startAutoRefresh(gb *GrowthBook) {
	apiHost, clientKey := gb.GetAPIInfo()
	key := makeKey(apiHost, clientKey)
	if cacheBackgroundSync && sseSupported[key] {
		if refreshChans[key] != nil {
			return
		}
		ch := make(chan cmd)
		refreshChans[key] = ch
		go refreshFromSSE(key, ch)
		ch <- cmd{t: subCmd, gb: gb}
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
	c.data = make(map[repositoryKey]*cacheEntry)
	// activeFetches.clear();
	// clearAutoRefresh();
	// cacheInitialized = false;
	// await updatePersistentCache();
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
