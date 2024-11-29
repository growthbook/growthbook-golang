package growthbook

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// Some test functions generate warnings in the log. We need to check
// the expected ones, and not miss any unexpected ones.

func handleExpectedWarnings(
	t *testing.T, test []interface{}, expectedWarnings map[string]int) {
	name, ok := test[0].(string)
	if !ok {
		t.Errorf("can't extract test name!")
	}
	warnings, ok := expectedWarnings[name]
	if ok {
		if len(testLog.errors) == 0 && len(testLog.warnings) == warnings {
			testLog.reset()
		} else {
			t.Errorf("expected log warning")
		}
	}
}

// Helper to round variation ranges for comparison with fixed test
// values.
func roundRanges(ranges []BucketRange) []BucketRange {
	result := make([]BucketRange, len(ranges))
	for i, r := range ranges {
		rmin := math.Round(r.Min*1000000) / 1000000
		rmax := math.Round(r.Max*1000000) / 1000000
		result[i] = BucketRange{rmin, rmax}
	}
	return result
}

// Helper to round floating point arrays for test comparison.
func round(vals []float64) []float64 {
	result := make([]float64, len(vals))
	for i, v := range vals {
		result[i] = math.Round(v*1000000) / 1000000
	}
	return result
}

// Logger to capture error and log messages.
type testLogger struct {
	errors   []string
	warnings []string
	info     []string
}

var testLog = testLogger{}

func (log *testLogger) allErrors() string {
	return strings.Join(log.errors, ", ")
}

func (log *testLogger) allWarnings() string {
	return strings.Join(log.warnings, ", ")
}

func (log *testLogger) allInfo() string {
	return strings.Join(log.info, ", ")
}

func (log *testLogger) reset() {
	log.errors = []string{}
	log.warnings = []string{}
	log.info = []string{}
}

func formatArgs(args ...interface{}) string {
	s := ""
	for i, a := range args {
		if i != 0 {
			s += " "
		}
		s += fmt.Sprint(a)
	}
	return s
}

func (log *testLogger) Error(msg string, args ...interface{}) {
	s := msg
	if len(args) > 0 {
		s += ": " + formatArgs(args...)
	}
	log.errors = append(log.errors, s)
}

func (log *testLogger) Errorf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	log.errors = append(log.errors, s)
}

func (log *testLogger) Warn(msg string, args ...interface{}) {
	s := msg
	if len(args) > 0 {
		s += ": " + formatArgs(args...)
	}
	log.warnings = append(log.warnings, s)
}

func (log *testLogger) Warnf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	log.warnings = append(log.warnings, s)
}

func (log *testLogger) Info(msg string, args ...interface{}) {
	s := msg
	if len(args) > 0 {
		s += ": " + fmt.Sprint(args...)
	}
	log.info = append(log.info, s)
}

func (log *testLogger) Infof(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	log.info = append(log.info, s)
}

// Polyfill from Go v1.20 sort.

func sortFind(n int, cmp func(int) int) (i int, found bool) {
	// The invariants here are similar to the ones in Search.
	// Define cmp(-1) > 0 and cmp(n) <= 0
	// Invariant: cmp(i-1) > 0, cmp(j) <= 0
	i, j := 0, n
	for i < j {
		h := int(uint(i+j) >> 1) // avoid overflow when computing h
		// i ≤ h < j
		if cmp(h) > 0 {
			i = h + 1 // preserves cmp(i-1) > 0
		} else {
			j = h // preserves cmp(j) <= 0
		}
	}
	// i == j, cmp(i-1) > 0 and cmp(j) <= 0
	return i, i < n && cmp(i) == 0
}
