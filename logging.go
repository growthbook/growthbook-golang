package growthbook

import (
	"golang.org/x/exp/slog"
)

// SetLogger sets up the logging interface used throughout.
func SetLogger(userLogger *slog.Logger) {
	logger = userLogger
}

// Global private logging interface.
var logger *slog.Logger = slog.Default()
