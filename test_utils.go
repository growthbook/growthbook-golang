package growthbook

import (
	"fmt"
	"math"
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
func roundRanges(ranges []Range) []Range {
	result := make([]Range, len(ranges))
	for i, r := range ranges {
		rmin := math.Round(r.Min*1000000) / 1000000
		rmax := math.Round(r.Max*1000000) / 1000000
		result[i] = Range{rmin, rmax}
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
	s := ""
	for i, e := range log.errors {
		if i != 0 {
			s += ", "
		}
		s += e
	}
	return s
}

func (log *testLogger) allWarnings() string {
	s := ""
	for i, e := range log.warnings {
		if i != 0 {
			s += ", "
		}
		s += e
	}
	return s
}

func (log *testLogger) allInfo() string {
	s := ""
	for i, e := range log.info {
		if i != 0 {
			s += ", "
		}
		s += e
	}
	return s
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
	// fmt.Println("ERROR: ", s)
}

func (log *testLogger) Errorf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	log.errors = append(log.errors, s)
	// fmt.Println("ERROR: ", s)
}

func (log *testLogger) Warn(msg string, args ...interface{}) {
	s := msg
	if len(args) > 0 {
		s += ": " + formatArgs(args...)
	}
	log.warnings = append(log.warnings, s)
	// fmt.Println("WARN: ", s)
}

func (log *testLogger) Warnf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	log.warnings = append(log.warnings, s)
	// fmt.Println("WARN: ", s)
}

func (log *testLogger) Info(msg string, args ...interface{}) {
	s := msg
	if len(args) > 0 {
		s += ": " + fmt.Sprint(args...)
	}
	log.info = append(log.info, s)
	// fmt.Println("INFO: ", s)
}

func (log *testLogger) Infof(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	log.info = append(log.info, s)
	// fmt.Println("INFO: ", s)
}
