package growthbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"golang.org/x/exp/slog"
)

// Some test functions generate warnings in the log. We need to check
// the expected ones, and not miss any unexpected ones.

func handleExpectedWarnings(
	t *testing.T, name string, expectedWarnings map[string]int) {
	warnings, ok := expectedWarnings[name]
	if ok {
		if len(testLogHandler.errors) == 0 && len(testLogHandler.warnings) == warnings {
			testLogHandler.reset()
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

var testLogHandler *handler = newHandler()

var testLog *slog.Logger = slog.New(testLogHandler)

// Log handler to capture error and warning messages.
type handler struct {
	errors     []map[string]any
	warnings   []map[string]any
	buf        *bytes.Buffer
	subHandler slog.Handler
}

func newHandler() *handler {
	buf := bytes.Buffer{}
	h := slog.NewJSONHandler(&buf, nil)
	return &handler{
		errors:     []map[string]any{},
		warnings:   []map[string]any{},
		buf:        &buf,
		subHandler: h,
	}
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.subHandler.Enabled(ctx, level)
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	err := h.subHandler.Handle(ctx, r)
	if err != nil {
		return err
	}
	v := map[string]any{}
	err = json.Unmarshal(h.buf.Bytes(), &v)
	h.buf.Reset()
	if err != nil {
		return err
	}
	level, ok := v["level"]
	if !ok {
		return errors.New("no level in log message")
	}
	switch level {
	case "ERROR":
		h.errors = append(h.errors, v)
	case "WARN":
		h.warnings = append(h.warnings, v)
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newSubHandler := h.subHandler.WithAttrs(attrs)
	return &handler{
		errors:     h.errors,
		warnings:   h.warnings,
		buf:        h.buf,
		subHandler: newSubHandler,
	}
}

func (h *handler) WithGroup(name string) slog.Handler {
	newSubHandler := h.subHandler.WithGroup(name)
	return &handler{
		errors:     h.errors,
		warnings:   h.warnings,
		buf:        h.buf,
		subHandler: newSubHandler,
	}
}

func (h *handler) reset() {
	h.errors = []map[string]any{}
	h.warnings = []map[string]any{}
	h.buf.Reset()
}

func (h *handler) allErrors() string {
	ss := []string{}
	for _, e := range h.errors {
		b, err := json.Marshal(e)
		if err == nil {
			ss = append(ss, string(b))
		}
	}
	return strings.Join(ss, ", ")
}

func (h *handler) allWarnings() string {
	ss := []string{}
	for _, e := range h.warnings {
		b, err := json.Marshal(e)
		if err == nil {
			ss = append(ss, string(b))
		}
	}
	return strings.Join(ss, ", ")
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
