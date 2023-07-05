package growthbook

import (
	"net/url"
	"os"
	"testing"
)

func TestIsURLTargetedNoTargetingRules(t *testing.T) {
	url := mustParseUrl("https://example.com/testing")
	if isURLTargeted(url, []URLTarget{}) != false {
		t.Error("expected isURLTargeted to return false")
	}
}

func TestIsURLTargetedMixIncludeExclude(t *testing.T) {
	urls := "https://www.example.com"
	url := mustParseUrl(urls)

	includeMatch := URLTarget{SimpleURLTarget, true, urls}
	excludeMatch := URLTarget{SimpleURLTarget, false, urls}
	includeNoMatch := URLTarget{SimpleURLTarget, true, "https://wrong.com"}
	excludeNoMatch := URLTarget{SimpleURLTarget, false, "https://another.com"}

	check := func(icase int, expected bool, targets ...URLTarget) {
		if isURLTargeted(url, targets) != expected {
			t.Errorf("%d: expected isURLTargets to return %v", icase, expected)
		}
	}

	// One include rule matches, one exclude rule matches
	check(1, false, includeMatch, includeNoMatch, excludeMatch, excludeNoMatch)

	// One include rule matches, no exclude rule matches
	check(2, true, includeMatch, includeNoMatch, excludeNoMatch)

	// No include rule matches, no exclude rule matches
	check(3, false, includeNoMatch, excludeNoMatch)

	// No include rule matches, one exclude rule matches
	check(4, false, includeNoMatch, excludeNoMatch, excludeMatch)

	// Only exclude rules, none matches
	check(5, true, excludeNoMatch, excludeNoMatch)

	// Only exclude rules, one matches
	check(6, false, excludeNoMatch, excludeMatch)

	// Only include rules, none matches
	check(7, false, includeNoMatch, includeNoMatch)

	// Only include rules, one matches
	check(8, true, includeNoMatch, includeMatch)
}

func TestIsURLTargetedExcludeOnTopOfInclude(t *testing.T) {
	rules := []URLTarget{
		{Include: true, Type: SimpleURLTarget, Pattern: "/search"},
		{Include: false, Type: SimpleURLTarget, Pattern: "/search?bad=true"},
	}

	check := func(icase int, expected bool, urls string) {
		if isURLTargeted(mustParseUrl(urls), rules) != expected {
			t.Errorf("%d: expected isURLTargets to return %v", icase, expected)
		}
	}

	check(1, true, "https://example.com/search")
	check(2, false, "https://example.com/search?bad=true")
	check(3, true, "https://example.com/search?good=true")
}

type urlTest struct {
	targetType URLTargetType
	url        string
	pattern    string
	expected   bool
}

var cases = []urlTest{
	{RegexURLTarget, "https://www.example.com/post/123", "^/post/[0-9]+", true},
	{RegexURLTarget, "https://www.example.com/post/abc", "^/post/[0-9]+", false},
	{RegexURLTarget, "https://www.example.com/new/post/123", "^/post/[0-9]+", false},
	{RegexURLTarget, "https://www.example.com/new/post/123", "example\\.com.*/post/[0-9]+", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "/foo", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "/foo?baz=2", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "/foo?foo=3", false},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "/bar?baz=2", false},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "foo", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "*?baz=2&bar=1", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "*.example.com/foo", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "blah.example.com/foo", false},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "https://www.*.com/foo", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "*.example.com", false},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "http://www.example.com/foo", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "f", false},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "f*", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "*f*", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "/foo/", true},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "/foo/bar", false},
	{SimpleURLTarget, "https://www.example.com/foo?bar=1&baz=2", "/bar/foo", false},
	{SimpleURLTarget, "https://www.example.com/foo/bar/baz", "/foo/*/baz", true},
	{SimpleURLTarget, "https://www.example.com/foo/bar/(baz", "/foo/*", true},
	{SimpleURLTarget, "https://www.example.com/foo/bar/#test", "/foo/*", true},
	{SimpleURLTarget, "https://www.example.com/foo/#test", "/foo/", true},
	{SimpleURLTarget, "https://www.example.com/foo/#test", "/foo/#test", true},
	{SimpleURLTarget, "https://www.example.com/foo/#test", "/foo/#blah", false},
	{SimpleURLTarget, "/foo/bar/?baz=1", "http://example.com/foo/bar", false},
	{SimpleURLTarget, "/foo/bar/?baz=1", "/foo/bar", true},
	{SimpleURLTarget, "&??*&&(", "/foo/bar", false},
	{SimpleURLTarget, "&??*&&(", "((*)(*$&#@!!)))", false},
}

func TestIsURLTargetedTableDriven(t *testing.T) {
	for itest, test := range cases {
		targets := []URLTarget{{test.targetType, true, test.pattern}}
		if isURLTargeted(mustParseUrl(test.url), targets) != test.expected {
			types := "simple"
			if test.targetType == RegexURLTarget {
				types = "regexp"
			}
			t.Errorf("%d: type=%s  url=%s  pattern=%s  expected=%v",
				itest+1, types, test.url, test.pattern, test.expected)
		}
	}
}

func mustParseUrl(u string) *url.URL {
	result, err := url.Parse(u)
	if err != nil {
		logError("Failed to parse URL: ", u)
		os.Exit(1)
	}
	return result
}
