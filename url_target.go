package growthbook

import (
	"net/url"
	"regexp"
	"strings"
)

// URLTargetType is the type of an URLTarget pattern.
type URLTargetType string

const (
	URLTargetSimple URLTargetType = "simple"
	URLTargetRegex  URLTargetType = "regex"
)

// URLTarget represents one URL targeting rule on an Experiment or FeatureRule.
// A nil Include is treated as include=true.
type URLTarget struct {
	Type    URLTargetType `json:"type"`
	Pattern string        `json:"pattern"`
	Include *bool         `json:"include"`
}

// isURLTargeted ports JS sdk-js' isURLTargeted: any matching exclude pattern
// rejects; if there are include rules, at least one must match; if there are
// only exclude rules and none match, the URL is considered targeted.
func isURLTargeted(rawURL string, targets []URLTarget) bool {
	if len(targets) == 0 {
		return false
	}
	hasInclude := false
	included := false
	for _, t := range targets {
		match := evalURLTarget(rawURL, t.Type, t.Pattern)
		include := t.Include == nil || *t.Include
		if !include {
			if match {
				return false
			}
			continue
		}
		hasInclude = true
		if match {
			included = true
		}
	}
	return included || !hasInclude
}

func evalURLTarget(rawURL string, typ URLTargetType, pattern string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch typ {
	case URLTargetRegex:
		return evalRegexURLTarget(parsed, pattern)
	case URLTargetSimple, "":
		return evalSimpleURLTarget(parsed, pattern)
	}
	return false
}

func evalRegexURLTarget(parsed *url.URL, pattern string) bool {
	// JS getUrlRegExp escapes unescaped `/` to support regex strings written for
	// `/.../"` literal style. Go's RE2 doesn't require slashes to be escaped, so
	// strip any backslash-escaped slashes first.
	clean := strings.ReplaceAll(pattern, `\/`, `/`)
	re, err := regexp.Compile(clean)
	if err != nil {
		return false
	}
	full := parsed.String()
	if re.MatchString(full) {
		return true
	}
	// Also try the URL with the origin (scheme://host) stripped so `/path?x=1`
	// patterns can match.
	origin := ""
	if parsed.Scheme != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	if origin != "" && strings.HasPrefix(full, origin) {
		return re.MatchString(full[len(origin):])
	}
	return false
}

var simpleSchemePrefixRe = regexp.MustCompile(`(?i)^([^:/?]*)\.`)

func evalSimpleURLTarget(actual *url.URL, pattern string) bool {
	// Add scheme if pattern starts with a bare host: `example.com/x` -> `https://example.com/x`.
	pattern = simpleSchemePrefixRe.ReplaceAllString(pattern, "https://$1.")
	pattern = strings.ReplaceAll(pattern, "*", "_____")

	expected, err := url.Parse(pattern)
	if err != nil {
		return false
	}
	// Mimic JS `new URL(pattern, "https://_____")` — fill in a base host if the
	// pattern parsed as a path-only URL.
	if expected.Host == "" {
		expected, err = url.Parse("https://_____" + ensureLeadingSlash(pattern))
		if err != nil {
			return false
		}
	}

	if !evalSimpleURLPart(actual.Host, expected.Host, false) {
		return false
	}
	if !evalSimpleURLPart(actual.Path, expected.Path, true) {
		return false
	}
	if expected.Fragment != "" && !evalSimpleURLPart(actual.Fragment, expected.Fragment, false) {
		return false
	}
	for k, vs := range expected.Query() {
		want := ""
		if len(vs) > 0 {
			want = vs[0]
		}
		if !evalSimpleURLPart(actual.Query().Get(k), want, false) {
			return false
		}
	}
	return true
}

func evalSimpleURLPart(actual, pattern string, isPath bool) bool {
	escaped := regexp.QuoteMeta(pattern)
	escaped = strings.ReplaceAll(escaped, "_____", ".*")
	if isPath {
		escaped = strings.TrimPrefix(escaped, "/")
		escaped = strings.TrimSuffix(escaped, "/")
		escaped = `/?` + escaped + `/?`
	}
	re, err := regexp.Compile(`(?i)^` + escaped + `$`)
	if err != nil {
		return false
	}
	return re.MatchString(actual)
}

func ensureLeadingSlash(s string) string {
	if strings.HasPrefix(s, "/") {
		return s
	}
	return "/" + s
}
