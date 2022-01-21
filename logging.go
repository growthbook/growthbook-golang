package growthbook

import (
	"fmt"
	"os"
)

// Message constants used for logging.
const (
	ErrCtxJSONFailedToParse        = "failed parsing JSON input for context"
	ErrExpJSONFailedToParse        = "failed parsing JSON input for experiment"
	ErrCtxJSONInvalidURL           = "invalid URL in JSON context data"
	ErrExpJSONInvalidCondition     = "invalid condition in JSON experiment data"
	WarnCtxJSONUnknownKey          = "unknown key in JSON context data"
	WarnExpJSONKeyNotSet           = "key not set in JSON experiment data"
	WarnExpCoverageMustBePositive  = "Experiment coverage must be greater than or equal to 0"
	WarnExpCoverageMustBeFraction  = "Experiment coverage must be less than or equal to 1"
	WarnExpWeightsWrongLength      = "Experiment weights and variations arrays must be the same length"
	WarnExpWeightsWrongTotal       = "Experiment weights must add up to 1"
	WarnRuleSkipHashAttributeType  = "Skip rule because of non-string hash attribute"
	InfoRuleSkipCondition          = "Skip rule because of condition"
	InfoRuleSkipNoHashAttribute    = "Skip rule because of missing hash attribute"
	InfoRuleSkipEmptyHashAttribute = "Skip rule because of empty hash attribute"
	InfoRuleSkipCoverage           = "Skip rule because of coverage"
	InfoRuleSkipUserNotInExp       = "Skip rule because user not in experiment"
)

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
		logger.Error(msg, args...)
	}
}

func logErrorf(format string, args ...interface{}) {
	if logger != nil {
		logger.Errorf(format, args...)
	}
}

func logWarn(msg string, args ...interface{}) {
	if logger != nil {
		logger.Warn(msg, args...)
	}
}

func logWarnf(format string, args ...interface{}) {
	if logger != nil {
		logger.Warnf(format, args...)
	}
}

func logInfo(msg string, args ...interface{}) {
	if logger != nil {
		logger.Info(msg, args...)
	}
}

func logInfof(format string, args ...interface{}) {
	if logger != nil {
		logger.Infof(format, args...)
	}
}

// DevLogger is a logger instance suitable for use in development. It
// prints all logged messages to standard output, and exits on errors.
type DevLogger struct{}

func (log *DevLogger) Error(msg string, args ...interface{}) {
	fmt.Print("[ERROR] ", msg)
	if len(args) > 0 {
		fmt.Print(": ")
		fmt.Println(args...)
	}
	os.Exit(1)
}

func (log *DevLogger) Errorf(format string, args ...interface{}) {
	fmt.Printf("[ERROR] "+format+"\n", args...)
	os.Exit(1)
}

func (log *DevLogger) Warn(msg string, args ...interface{}) {
	fmt.Print("[WARNING] ", msg)
	if len(args) > 0 {
		fmt.Print(": ")
		fmt.Println(args...)
	}
}

func (log *DevLogger) Warnf(format string, args ...interface{}) {
	fmt.Printf("[WARNING] "+format+"\n", args...)
}

func (log *DevLogger) Info(msg string, args ...interface{}) {
	fmt.Print("[INFO] ", msg)
	if len(args) > 0 {
		fmt.Print(": ")
		fmt.Println(args...)
	}
}

func (log *DevLogger) Infof(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}
