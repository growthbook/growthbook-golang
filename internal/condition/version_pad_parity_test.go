package condition

import (
	"regexp"
	"strings"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

// regexPaddedVersionString is a regex implementation of the JS SDK's
// paddedVersionString, kept as a reference for the byte-scanning one. Like JS,
// it drops everything from the first "+", even across a newline.
func regexPaddedVersionString(input value.Value) string {
	var version string
	switch v := input.(type) {
	case value.NumValue, value.StrValue:
		version = v.String()
	}
	if version == "" {
		version = "0"
	}
	version = regexp.MustCompile(`(?s)(^v|\+.*$)`).ReplaceAllString(version, "")
	parts := regexp.MustCompile(`[-.]`).Split(version, -1)
	if len(parts) == 3 {
		parts = append(parts, "~")
	}
	for i, p := range parts {
		if regexp.MustCompile(`^[0-9]+$`).MatchString(p) && len(p) < 5 {
			parts[i] = strings.Repeat(" ", 5-len(p)) + p
		}
	}
	return strings.Join(parts, "-")
}

var versionPadInputs = []string{
	"", "0", "1", "1.2", "1.2.3", "v1.2.3", "vv1.2.3", "V1.2.3", "v", "+", "v+",
	"1.2.3+build", "1.2.3+a+b", "1.2.3+build-1.2", "+1.2.3", "1.2.3-rc.1+b",
	"1..2", ".", "-", "1.2.3.", "-1.2.3", "1.2.3--", "1.0.0-0.0.0",
	"12345.123456.1234", "0001.02.3", "1.2.3-alpha.10", "1.2.3-ALPHA",
	"1.2.3-a b", "1.2.3~", "٣.2.1", "1.2.3-é", "+\n", "1.2.3+b\nx", "1.2.3\n",
}

func TestPaddedVersionStringDropsBuildAcrossNewline(t *testing.T) {
	require.Equal(t, "    1-    2-    3-~", paddedVersionString(value.New("1.2.3+b\nx")))
}

func TestPaddedVersionStringMatchesRegexVersion(t *testing.T) {
	for _, s := range versionPadInputs {
		require.Equal(t, regexPaddedVersionString(value.New(s)), paddedVersionString(value.New(s)), "input %q", s)
	}
	for _, n := range []float64{0, 1, 1.5, 10, 123456, -1} {
		require.Equal(t, regexPaddedVersionString(value.New(n)), paddedVersionString(value.New(n)), "input %v", n)
	}
}

func FuzzPaddedVersionString(f *testing.F) {
	for _, s := range versionPadInputs {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if regexPaddedVersionString(value.New(s)) != paddedVersionString(value.New(s)) {
			t.Errorf("mismatch for %q", s)
		}
	})
}

func BenchmarkPaddedVersionString(b *testing.B) {
	v := value.New("v1.20.3-rc.1+build.5")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = paddedVersionString(v)
	}
}
