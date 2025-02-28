package growthbook

import (
	"net/url"
	"strconv"
)

// Checks if an experiment variation is being forced via a URL query
// string.
//
// As an example, if the id is "my-test" and url is
// http://localhost/?my-test=1, this function returns 1.
func getQueryStringOverride(id string, url *url.URL, numVariations int) (int, bool) {
	if url == nil {
		return 0, false
	}

	v, ok := url.Query()[id]
	if !ok || len(v) > 1 {
		return 0, false
	}

	vi, err := strconv.Atoi(v[0])
	if err != nil {
		return 0, false
	}

	if vi < 0 || vi >= numVariations {
		return 0, false
	}

	return vi, true
}

func if0(v1 int, v2 int) int {
	if v1 == 0 {
		return v2
	}
	return v1
}
