package growthbook

import (
	"context"
	"log/slog"
	"math"
	"os"
	"sync"
	"testing"
)

func debugLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func testLogger(level slog.Level, t *testing.T) (*slog.Logger, *[]logEntry) {
	t.Helper()
	handler := &logCollectorHandler{
		level: level,
		logs:  make([]logEntry, 0),
	}
	logger := slog.New(handler)

	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		handler.mu.Lock()
		defer handler.mu.Unlock()
		if len(handler.logs) == 0 {
			return
		}
		t.Helper()
		t.Log("Logs:")
		for _, e := range handler.logs {
			t.Log(e.Level, e.Message)
		}
	})
	return logger, &handler.logs
}

type logEntry struct {
	Level   string
	Message string
}

type logCollectorHandler struct {
	mu    sync.Mutex
	logs  []logEntry
	level slog.Level
}

func (h *logCollectorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *logCollectorHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logs = append(h.logs, recordToLogEntry(record))
	return nil
}

func (h *logCollectorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *logCollectorHandler) WithGroup(name string) slog.Handler {
	return h
}

// Преобразование slog.Record в logEntry
func recordToLogEntry(record slog.Record) logEntry {
	entry := logEntry{
		Level:   record.Level.String(),
		Message: record.Message,
	}
	return entry
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
func roundArr(vals []float64) []float64 {
	result := make([]float64, len(vals))
	for i, v := range vals {
		result[i] = math.Round(v*1000000) / 1000000
	}
	return result
}
