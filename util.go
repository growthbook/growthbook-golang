package growthbook

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
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

var (
	versionStripRe = regexp.MustCompile(`(^v|\+.*$)`)
	versionSplitRe = regexp.MustCompile(`[-.]`)
	versionNumRe   = regexp.MustCompile(`^[0-9]+$`)
)

func paddedVersionString(input string) string {
	// Remove build info and leading `v` if any
	// Split version into parts (both core version numbers and pre-release tags)
	// "v1.2.3-rc.1+build123" -> ["1","2","3","rc","1"]
	stripped := versionStripRe.ReplaceAllLiteralString(input, "")
	parts := versionSplitRe.Split(stripped, -1)

	// If it's SemVer without a pre-release, add `~` to the end
	// ["1","0","0"] -> ["1","0","0","~"]
	// "~" is the largest ASCII character, so this will make "1.0.0"
	// greater than "1.0.0-beta" for example
	if len(parts) == 3 {
		parts = append(parts, "~")
	}

	// Left pad each numeric part with spaces so string comparisons will
	// work ("9">"10", but " 9"<"10")
	for i := range parts {
		if versionNumRe.MatchString(parts[i]) {
			parts[i] = strings.Repeat(" ", 5-len(parts[i])) + parts[i]
		}
	}
	// Then, join back together into a single string
	return strings.Join(parts, "-")
}

func if0(v1 int, v2 int) int {
	if v1 == 0 {
		return v2
	}
	return v1
}
