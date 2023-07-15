package growthbook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
)

// LogLevel is an enumeration for log message levels.
type LogLevel int

const (
	Debug LogLevel = iota
	Info           = iota
	Warn           = iota
	Error          = iota
)

// Convert log level to a string for simple logging.
func (lev LogLevel) String() string {
	switch lev {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	}
	return "<unknown>"
}

// LogMsg is an enumeration for log message types.
type LogMsg int

const (
	ConditionTypeMismatch                   LogMsg = iota
	CoverageOutOfRange                             = iota
	EmptyHashAttribute                             = iota
	ExperimentDisabled                             = iota
	ExperimentForcedVariation                      = iota
	ExperimentForceViaQueryString                  = iota
	ExperimentInvalid                              = iota
	ExperimentSkipCondition                        = iota
	ExperimentSkipCoverage                         = iota
	ExperimentSkipFilters                          = iota
	ExperimentSkipGroups                           = iota
	ExperimentSkipInactive                         = iota
	ExperimentSkipIncludeFunction                  = iota
	ExperimentSkipInvalidHashVersion               = iota
	ExperimentSkipMissingHashAttribute             = iota
	ExperimentSkipNamespace                        = iota
	ExperimentSkipStopped                          = iota
	ExperimentSkipURL                              = iota
	ExperimentSkipURLTargeting                     = iota
	ExperimentWeightsTotal                         = iota
	ExperimentWeightVariationLengthMismatch        = iota
	FailedDecrypt                                  = iota
	FeatureForceFromRule                           = iota
	FeatureGlobalOverride                          = iota
	FeatureSkipCondition                           = iota
	FeatureSkipFilters                             = iota
	FeatureSkipInvalidRule                         = iota
	FeatureSkipUserRollout                         = iota
	FeatureUnknown                                 = iota
	FeatureUseDefaultValue                         = iota
	InExperiment                                   = iota
	SSEConnecting                                  = iota
	SSEError                                       = iota
	SSEMultipleErrors                              = iota
	SSENewData                                     = iota
	SSEStreamDisconnect                            = iota
	SSEWaitingToReconnect                          = iota
)

func (msg LogMsg) Label() string {
	switch msg {
	case ConditionTypeMismatch:
		return "ConditionTypeMismatch"
	case CoverageOutOfRange:
		return "CoverageOutOfRange"
	case EmptyHashAttribute:
		return "EmptyHashAttribute"
	case ExperimentDisabled:
		return "ExperimentDisabled"
	case ExperimentForcedVariation:
		return "ExperimentForcedVariation"
	case ExperimentForceViaQueryString:
		return "ExperimentForceViaQueryString"
	case ExperimentInvalid:
		return "ExperimentInvalid"
	case ExperimentSkipCondition:
		return "ExperimentSkipCondition"
	case ExperimentSkipCoverage:
		return "ExperimentSkipCoverage"
	case ExperimentSkipFilters:
		return "ExperimentSkipFilters"
	case ExperimentSkipGroups:
		return "ExperimentSkipGroups"
	case ExperimentSkipInactive:
		return "ExperimentSkipInactive"
	case ExperimentSkipIncludeFunction:
		return "ExperimentSkipIncludeFunction"
	case ExperimentSkipInvalidHashVersion:
		return "ExperimentSkipInvalidHashVersion"
	case ExperimentSkipMissingHashAttribute:
		return "ExperimentSkipMissingHashAttribute"
	case ExperimentSkipNamespace:
		return "ExperimentSkipNamespace"
	case ExperimentSkipStopped:
		return "ExperimentSkipStopped"
	case ExperimentSkipURL:
		return "ExperimentSkipURL"
	case ExperimentSkipURLTargeting:
		return "ExperimentSkipURLTargeting"
	case ExperimentWeightsTotal:
		return "ExperimentWeightsTotal"
	case ExperimentWeightVariationLengthMismatch:
		return "ExperimentWeightVariationLengthMismatch"
	case FailedDecrypt:
		return "FailedDecrypt"
	case FeatureForceFromRule:
		return "FeatureForceFromRule"
	case FeatureGlobalOverride:
		return "FeatureGlobalOverride"
	case FeatureSkipCondition:
		return "FeatureSkipCondition"
	case FeatureSkipFilters:
		return "FeatureSkipFilters"
	case FeatureSkipInvalidRule:
		return "FeatureSkipInvalidRule"
	case FeatureSkipUserRollout:
		return "FeatureSkipUserRollout"
	case FeatureUnknown:
		return "FeatureUnknown"
	case FeatureUseDefaultValue:
		return "FeatureUseDefaultValue"
	case InExperiment:
		return "InExperiment"
	case SSEConnecting:
		return "SSEConnecting"
	case SSEError:
		return "SSEError"
	case SSEMultipleErrors:
		return "SSEMultipleErrors"
	case SSENewData:
		return "SSENewData"
	case SSEStreamDisconnect:
		return "SSEStreamDisconnect"
	case SSEWaitingToReconnect:
		return "SSEWaitingToReconnect"
	default:
		return "<unknown log message>"
	}
}

// Return message template for a log message.
func (msg LogMsg) template() *template.Template {
	expSkip := "Skip because of %s (key = {{.key}})"
	t := ""
	switch msg {
	case ConditionTypeMismatch:
		t = "Types don't match in condition comparison operation"
	case CoverageOutOfRange:
		t = "Experiment coverage must be in the range [0, 1]"
	case EmptyHashAttribute:
		t = "Skip because of empty hash attribute"
	case ExperimentDisabled:
		t = "Context disabled (key = {{.key}})"
	case ExperimentForcedVariation:
		t = "Forced variation (key = {{.key}}): {{.force}}"
	case ExperimentForceViaQueryString:
		t = "Force via querystring (key = {{.key}}): {{.qsOverride}}"
	case ExperimentInvalid:
		t = "Invalid experiment (key = {{.key}})"
	case ExperimentSkipCondition:
		t = fmt.Sprintf(expSkip, "condition")
	case ExperimentSkipCoverage:
		t = fmt.Sprintf(expSkip, "coverage")
	case ExperimentSkipFilters:
		t = fmt.Sprintf(expSkip, "filters")
	case ExperimentSkipGroups:
		t = fmt.Sprintf(expSkip, "groups")
	case ExperimentSkipInactive:
		t = "Skip because inactive (key = {{.key}})"
	case ExperimentSkipIncludeFunction:
		t = fmt.Sprintf(expSkip, "include function")
	case ExperimentSkipInvalidHashVersion:
		t = fmt.Sprintf(expSkip, "invalid hash version")
	case ExperimentSkipMissingHashAttribute:
		t = fmt.Sprintf(expSkip, "missing hash attribute")
	case ExperimentSkipNamespace:
		t = fmt.Sprintf(expSkip, "namespace")
	case ExperimentSkipStopped:
		t = "Skip because stopped (key = {{.key}})"
	case ExperimentSkipURL:
		t = fmt.Sprintf(expSkip, "URL")
	case ExperimentSkipURLTargeting:
		t = fmt.Sprintf(expSkip, "URL targeting")
	case ExperimentWeightsTotal:
		t = "Experiment weights must add up to 1"
	case ExperimentWeightVariationLengthMismatch:
		t = "Experiment weights and variations arrays must be the same length"
	case FailedDecrypt:
		t = "Failed to decrypt encrypted features"
	case FeatureForceFromRule:
		t = "Force value from rule (id = {{.id}}): {{.rule}}"
	case FeatureGlobalOverride:
		t = "Global override (id = {{.id}}, override = {{.override}})"
	case FeatureSkipCondition:
		t = "Skip rule because of condition (id = {{.id}}): {{.rule}}"
	case FeatureSkipFilters:
		t = "Skip rule because of filters (id = {{.id}}): {{.rule}}"
	case FeatureSkipInvalidRule:
		t = "Skip invalid rule (id = {{.id}}): {{.rule}}"
	case FeatureSkipUserRollout:
		t = "Skip rule because user not included in rollout (id = {{.id}}): {{.rule}}"
	case FeatureUnknown:
		t = "Unknown feature: {{.feature}}"
	case FeatureUseDefaultValue:
		t = "Use default value (id = {{.id}}): {{.value}}"
	case InExperiment:
		t = "In experiment (key = {{.key}}): variation ID {{.variationID}}"
	case SSEConnecting:
		t = "Connecting to SSE stream: {{.apiHost}}"
	case SSEError:
		t = "SSE error ({{.key}}): {{.error}}"
	case SSEMultipleErrors:
		t = "Multiple SSE errors: disconnecting stream: {{.key}}"
	case SSENewData:
		t = "New feature data from SSE stream"
	case SSEStreamDisconnect:
		t = "SSE event stream disconnected: {{.key}}"
	case SSEWaitingToReconnect:
		t = "Waiting to reconnect SSE stream: {{.key}} (delaying {{.delay}})"
	default:
		return nil
	}
	tmpl, err := template.New("log").Parse(t)
	if err == nil {
		return tmpl
	}
	return nil
}

// LogData provides detail data for log messages.
type LogData map[string]interface{}

// JSONLog is a wrapper type used to control rendering of logging
// arguments to JSON strings when it's needed.
type JSONLog struct{ value interface{} }

// Convert JSONLog arguments in a log message into JSONified string
// values.
func (data LogData) FixJSONArgs() LogData {
	retargs := LogData{}
	for k, v := range data {
		jsonv, ok := v.(JSONLog)
		if !ok {
			retargs[k] = v
			continue
		}
		d, err := json.Marshal(jsonv.value)
		if err == nil {
			retargs[k] = string(d)
		} else {
			retargs[k] = v
		}
	}
	return retargs
}

// LogMessage represents a single log message, with a level (error,
// warn, info) and message type and detail data to go with it.
type LogMessage struct {
	Level   LogLevel
	Message LogMsg
	Data    LogData
}

// Convert a log message to a string for simple logging applications.
func (msg *LogMessage) String() string {
	levelPrefix := "[" + msg.Level.String() + "] "

	tmpl := msg.Message.template()
	if tmpl == nil {
		return levelPrefix + "<uninterpretable log message>"
	}

	var buff bytes.Buffer
	args := msg.Data
	if args == nil {
		args = LogData{}
	}
	if err := tmpl.Execute(&buff, args.FixJSONArgs()); err != nil {
		return levelPrefix + "<log message with invalid formatting>"
	}

	return levelPrefix + buff.String()
}

// Logger is a common interface for logging information and warning
// messages (errors are returned directly by SDK functions, but there
// is some useful "out of band" data that's provided via this
// interface).
type Logger interface {
	Handle(msg *LogMessage)
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

// DevLogger is a logger instance suitable for use in development. It
// prints all logged messages to standard output, and exits on errors.
type DevLogger struct{}

func (log DevLogger) Handle(msg *LogMessage) {
	fmt.Println(msg.String())
}

// Internal logging functions wired up to logging interface.

func logError(msg LogMsg, args LogData) {
	if logger != nil {
		logger.Handle(&LogMessage{Error, msg, args})
	}
}

func logWarn(msg LogMsg, args LogData) {
	if logger != nil {
		logger.Handle(&LogMessage{Warn, msg, args})
	}
}
func logInfo(msg LogMsg, args LogData) {
	if logger != nil {
		logger.Handle(&LogMessage{Info, msg, args})
	}
}
