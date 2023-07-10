package growthbook

import (
	"encoding/json"
	"fmt"
)

type JSONLog struct{ value interface{} }

// Logger is a common interface for logging information and warning
// messages (errors are returned directly by SDK functions, but there
// is some useful "out of band" data that's provided via this
// interface).
type Logger interface {
	Error(msg string, args ...interface{})
	Errorf(format string, args ...interface{})
	Warn(msg string, args ...interface{})
	Warnf(format string, args ...interface{})
	Info(msg string, args ...interface{})
	Infof(format string, args ...interface{})
}

// SetLogger sets up the logging interface used throughout. The idea
// here is to provide developers with the option of handling errors
// and warnings in a strict way during development and a lenient way
// in production. For example, in development, setting a logger that
// prints a message for all logged output and panics on any logged
// warning or error might be appropriate, while in production, it
// would make more sense to log only warnings and errors and to
// proceed without halting. All GrowthBook SDK functions leave values
// in a sensible default state after errors, so production systems can
// essentially ignore any errors.
func SetLogger(userLogger Logger) {
	logger = userLogger
}

// Global private logging interface.
var logger Logger

// Internal logging functions wired up to logging interface.

func logError(msg string, args ...interface{}) {
	if logger != nil {
		logger.Error(msg, fixJSONArgs(args)...)
	}
}

func logErrorf(format string, args ...interface{}) {
	if logger != nil {
		logger.Errorf(format, fixJSONArgs(args)...)
	}
}

func logWarn(msg string, args ...interface{}) {
	if logger != nil {
		logger.Warn(msg, fixJSONArgs(args)...)
	}
}

func logWarnf(format string, args ...interface{}) {
	if logger != nil {
		logger.Warnf(format, fixJSONArgs(args)...)
	}
}

func logInfo(msg string, args ...interface{}) {
	if logger != nil {
		logger.Info(msg, fixJSONArgs(args)...)
	}
}

func logInfof(format string, args ...interface{}) {
	if logger != nil {
		logger.Infof(format, fixJSONArgs(args)...)
	}
}

// DevLogger is a logger instance suitable for use in development. It
// prints all logged messages to standard output, and exits on errors.
type DevLogger struct{}

func (log *DevLogger) Error(msg string, args ...interface{}) {
	fmt.Print("[ERROR] ", msg)
	handlArgs(args)
}

func (log *DevLogger) Errorf(format string, args ...interface{}) {
	fmt.Printf("[ERROR] "+format+"\n", fixJSONArgs(args)...)
}

func (log *DevLogger) Warn(msg string, args ...interface{}) {
	fmt.Print("[WARNING] ", msg)
	handlArgs(args)
}

func (log *DevLogger) Warnf(format string, args ...interface{}) {
	fmt.Printf("[WARNING] "+format+"\n", fixJSONArgs(args)...)
}

func (log *DevLogger) Info(msg string, args ...interface{}) {
	fmt.Print("[INFO] ", msg)
	handlArgs(args)
}

func (log *DevLogger) Infof(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", fixJSONArgs(args)...)
}

func handlArgs(args []interface{}) {
	if len(args) == 0 {
		fmt.Println()
		return
	}
	fmt.Print(": ")
	fmt.Println(fixJSONArgs(args)...)
}

func fixJSONArgs(args []interface{}) []interface{} {
	retargs := make([]interface{}, len(args))
	for i, v := range args {
		jsonv, ok := v.(JSONLog)
		if !ok {
			retargs[i] = v
			continue
		}
		d, err := json.Marshal(jsonv.value)
		if err == nil {
			retargs[i] = string(d)
		} else {
			retargs[i] = v
		}
	}
	return retargs
}
