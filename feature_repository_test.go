package growthbook

import (
	"strings"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
)

func mockAPI(data *FeatureAPIResponse, sse bool) {
	mockAPIWithDelay(data, sse, 50)
}

func mockAPIWithDelay(data *FeatureAPIResponse, sse bool, delay time.Duration) {
	responder, err := httpmock.NewJsonResponder(200, data)
	if err != nil {
		logError("INTERNAL ERROR: couldn't create mock responder")
	}
	if delay != 0 {
		responder = responder.Delay(delay)
	}
	httpmock.RegisterResponder(
		"GET",
		"=~https://fakeapi.sample.io/api/features/.*",
		responder,
	)
}

func realURLs() map[string]int {
	result := make(map[string]int)
	for k, count := range httpmock.GetCallCountInfo() {
		ksplit := strings.Split(k, " ")
		url := ksplit[1]
		if strings.HasPrefix(url, "=~") {
			continue
		}
		result[url] = count
	}
	return result
}

// func TestRepoBasicRetrievalViaMock(t *testing.T) {
// 	httpmock.Activate()
// 	defer httpmock.DeactivateAndReset()

// 	response := FeatureAPIResponse{
// 		Features: map[string]*Feature{
// 			"foo": {DefaultValue: "initial"},
// 		},
// 	}
// 	mockAPI(&response, false)
//	httpmock.ZeroCallCounters()

// 	context := NewContext().
// 		WithAPIHost("https://fakeapi.sample.io").
// 		WithClientKey("sdk-TESTCLIENT")
// 	gb := New(context)

// 	apiResponse := gb.fetchFeatures()
// 	for k, v := range apiResponse.Features {
// 		fmt.Printf("%s: %#v\n", k, v)
// 	}
// 	fmt.Println(testLog.allErrors())
// 	fmt.Println(testLog.allWarnings())

// 	fmt.Println("httpmock.GetTotalCallCount() = ", httpmock.GetTotalCallCount())
// 	fmt.Println("httpmock.GetCallCountInfo() = ", httpmock.GetCallCountInfo())
// 	fmt.Println("realURLs() = ", realURLs())
// }

func setup() {
	httpmock.Activate()

	response := FeatureAPIResponse{
		Features: map[string]*Feature{
			"foo": {DefaultValue: "initial"},
		},
	}
	mockAPI(&response, false)
	httpmock.ZeroCallCounters()
	ConfigureCacheStaleTTL(100 * time.Millisecond)
}

func makeGB(clientKey string) *GrowthBook {
	context := NewContext().
		WithAPIHost("https://fakeapi.sample.io").
		WithClientKey(clientKey)
	return New(context)
}

func checkFeature(t *testing.T, gb *GrowthBook, feature string, expected interface{}) {
	value := gb.EvalFeature(feature).Value
	if value != expected {
		t.Errorf("feature value, expected %v, got %v", expected, value)
	}
}

func checkCalls(t *testing.T, expected int) {
	value := httpmock.GetTotalCallCount()
	if value != expected {
		t.Errorf("Expected %d calls to API, got %d", expected, value)
	}
}

func TestRepoDebounceFetchRequests(t *testing.T) {
	setup()
	defer httpmock.DeactivateAndReset()

	cache.clear()

	gb1 := makeGB("qwerty1234")
	gb2 := makeGB("other")
	gb3 := makeGB("qwerty1234")

	gb1.LoadFeatures(nil)
	gb2.LoadFeatures(nil)
	gb3.LoadFeatures(nil)

	checkCalls(t, 2)
	urls := realURLs()
	if urls["https://fakeapi.sample.io/api/features/other"] != 1 ||
		urls["https://fakeapi.sample.io/api/features/qwerty1234"] != 1 {
		t.Errorf("unexpected URL calls: %v", urls)
	}

	checkFeature(t, gb1, "foo", "initial")
	checkFeature(t, gb2, "foo", "initial")
	checkFeature(t, gb3, "foo", "initial")

	cache.clear()
}

func TestRepoUsesCacheAndCanRefreshManually(t *testing.T) {
	setup()
	defer httpmock.DeactivateAndReset()

	cache.clear()

	gb := makeGB("qwerty1234")
	time.Sleep(20 * time.Millisecond)

	// Initial value of feature should be null.
	checkFeature(t, gb, "foo", nil)
	checkCalls(t, 1)

	// Once features are loaded, value should be from the fetch request.
	gb.LoadFeatures(nil)
	checkFeature(t, gb, "foo", "initial")
	checkCalls(t, 1)

	// Value changes in API
	response := FeatureAPIResponse{
		Features: map[string]*Feature{
			"foo": {DefaultValue: "changed"},
		},
	}
	mockAPI(&response, false)

	// New instances should get cached value
	gb2 := makeGB("qwerty1234")
	checkFeature(t, gb2, "foo", nil)
	gb2.LoadFeatures(&FeatureRepoOptions{AutoRefresh: true})
	checkFeature(t, gb2, "foo", "initial")

	// Instance without autoRefresh.
	gb3 := makeGB("qwerty1234")
	checkFeature(t, gb3, "foo", nil)
	gb3.LoadFeatures(nil)
	checkFeature(t, gb3, "foo", "initial")

	checkCalls(t, 1)

	// Old instances should also get cached value.
	checkFeature(t, gb, "foo", "initial")

	// Refreshing while cache is fresh should not cause a new network
	// request.
	gb.RefreshFeatures(nil)
	checkCalls(t, 1)

	// Wait a bit for cache to become stale and refresh again.
	time.Sleep(100 * time.Millisecond)
	gb.RefreshFeatures(nil)
	checkCalls(t, 2)

	// The instance being updated should get the new value.
	checkFeature(t, gb, "foo", "changed")

	// The instance with auto-refresh should now have the new value.
	// TODO: AUTO-REFRESH!
	checkFeature(t, gb2, "foo", "changed")

	// The instance without auto-refresh should continue to have the old
	// value.
	checkFeature(t, gb3, "foo", "initial")

	// New instances should get the new value
	gb4 := makeGB("qwerty1234")
	checkFeature(t, gb4, "foo", nil)
	gb4.LoadFeatures(nil)
	checkFeature(t, gb4, "foo", "changed")

	checkCalls(t, 2)

	cache.clear()
}

func TestRepoUsesLocalStorageCache(t *testing.T) {

}

func TestRepoUpdatesFeaturesBasedOnSSE(t *testing.T) {

}

func TestRepoDoesntCacheWhenDevModeOn(t *testing.T) {

}

func TestRepoExposesAReadyFlag(t *testing.T) {

}

func TestRepoHandlesBrokenFetchResponses(t *testing.T) {

}

func TestRepoHandlesSuperLongAPIRequests(t *testing.T) {

}

func TestRepoHandlesSSEErrors(t *testing.T) {

}

func TestRepoHandlesLocalStorageerrors(t *testing.T) {

}

func TestRepoDoesntDoBackgroundSyncWhenDisabled(t *testing.T) {

}

func TestRepoDecryptsFeatures(t *testing.T) {

}
