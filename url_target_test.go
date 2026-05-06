package growthbook

import (
	"net/url"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestIsURLTargeted_Empty(t *testing.T) {
	if isURLTargeted("https://example.com/", nil) {
		t.Fatal("empty targets must return false")
	}
}

func TestIsURLTargeted_SimpleInclude(t *testing.T) {
	targets := []URLTarget{{Type: URLTargetSimple, Pattern: "https://example.com/foo"}}
	if !isURLTargeted("https://example.com/foo", targets) {
		t.Fatal("exact match should be targeted")
	}
	if isURLTargeted("https://example.com/bar", targets) {
		t.Fatal("non-matching path should not be targeted")
	}
}

func TestIsURLTargeted_SimpleWildcard(t *testing.T) {
	targets := []URLTarget{{Type: URLTargetSimple, Pattern: "https://*.example.com/path/*"}}
	if !isURLTargeted("https://app.example.com/path/x", targets) {
		t.Fatal("wildcard host + path should match")
	}
	if !isURLTargeted("https://api.example.com/path/y/z", targets) {
		t.Fatal("trailing wildcard should match deeper paths")
	}
	if isURLTargeted("https://example.com/path/x", targets) {
		t.Fatal("missing subdomain should not match wildcard host")
	}
}

func TestIsURLTargeted_SimpleSchemelessPattern(t *testing.T) {
	targets := []URLTarget{{Type: URLTargetSimple, Pattern: "example.com/foo"}}
	if !isURLTargeted("https://example.com/foo", targets) {
		t.Fatal("scheme should be added to bare host pattern")
	}
}

func TestIsURLTargeted_SimpleQueryParams(t *testing.T) {
	targets := []URLTarget{{Type: URLTargetSimple, Pattern: "https://example.com/?a=1&b=2"}}
	if !isURLTargeted("https://example.com/?b=2&a=1", targets) {
		t.Fatal("query params should be order-independent")
	}
	if isURLTargeted("https://example.com/?a=1", targets) {
		t.Fatal("missing required query param should not match")
	}
}

func TestIsURLTargeted_RegexInclude(t *testing.T) {
	targets := []URLTarget{{Type: URLTargetRegex, Pattern: `^https://example\.com/foo/\d+$`}}
	if !isURLTargeted("https://example.com/foo/42", targets) {
		t.Fatal("regex should match")
	}
	if isURLTargeted("https://example.com/foo/abc", targets) {
		t.Fatal("regex should reject non-digit")
	}
}

func TestIsURLTargeted_RegexJSEscapedSlashes(t *testing.T) {
	targets := []URLTarget{{Type: URLTargetRegex, Pattern: `^https:\/\/example\.com\/foo$`}}
	if !isURLTargeted("https://example.com/foo", targets) {
		t.Fatal("JS-style backslash-escaped slashes should be normalized")
	}
}

func TestIsURLTargeted_ExcludeWins(t *testing.T) {
	targets := []URLTarget{
		{Type: URLTargetSimple, Pattern: "https://example.com/*"},
		{Type: URLTargetSimple, Pattern: "https://example.com/admin", Include: boolPtr(false)},
	}
	if !isURLTargeted("https://example.com/page", targets) {
		t.Fatal("non-excluded URL inside include should match")
	}
	if isURLTargeted("https://example.com/admin", targets) {
		t.Fatal("excluded URL should reject even with matching include")
	}
}

func TestIsURLTargeted_OnlyExcludesNoMatch(t *testing.T) {
	targets := []URLTarget{
		{Type: URLTargetSimple, Pattern: "https://example.com/admin", Include: boolPtr(false)},
	}
	// JS behavior: with only excludes and no match, URL is targeted.
	if !isURLTargeted("https://example.com/page", targets) {
		t.Fatal("only excludes, none matching, should be targeted")
	}
	if isURLTargeted("https://example.com/admin", targets) {
		t.Fatal("excluded URL should not be targeted")
	}
}

func TestIsURLTargeted_NoIncludeMatched(t *testing.T) {
	targets := []URLTarget{
		{Type: URLTargetSimple, Pattern: "https://example.com/foo"},
	}
	if isURLTargeted("https://example.com/bar", targets) {
		t.Fatal("when an include exists but none match, URL is not targeted")
	}
}

func TestIsLegacyURLTargeted(t *testing.T) {
	u := mustParseURL(t, "https://example.com/path/to/page?step=1#top")
	if !isLegacyURLTargeted(u, `^https://example\.com/path/to/page\?step=1#top$`) {
		t.Fatal("legacy URL should match full URL")
	}
	if !isLegacyURLTargeted(u, `^/path/to/page\?step=1#top$`) {
		t.Fatal("legacy URL should match path-only URL")
	}
	if isLegacyURLTargeted(u, `^/other$`) {
		t.Fatal("legacy URL should reject non-matching URL")
	}
	if isLegacyURLTargeted(nil, `.*`) {
		t.Fatal("legacy URL should reject missing client URL")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
