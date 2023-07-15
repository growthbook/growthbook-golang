package growthbook

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func check(t *testing.T, level LogLevel, msg LogMsg, data LogData, expected string) {
	m := LogMessage{level, msg, data}
	result := m.String()
	if result != expected {
		t.Errorf("unexpected log conversion: '%s', should be '%s", result, expected)
	}
}

const featureJson = `{
  "defaultValue": 2,
	"rules": [
		{
			"force": 1,
			"condition": {
				"country": { "$in": ["US", "CA"] },
				"browser": "firefox"
			}
		}
	]
}`

func TestLogMessageConversion(t *testing.T) {
	check(t, Warn, FeatureUnknown,
		LogData{"feature": "test-feature"},
		"[WARN] Unknown feature: test-feature")

	check(t, Info, SSEConnecting,
		LogData{"apiHost": "test-api-host"},
		"[INFO] Connecting to SSE stream: test-api-host")

	check(t, Info, ExperimentForcedVariation,
		LogData{"key": "test-key", "force": 123},
		"[INFO] Forced variation (key = test-key): 123")

	check(t, Info, ExperimentForceViaQueryString,
		LogData{"key": "test-key", "qsOverride": 123},
		"[INFO] Force via querystring (key = test-key): 123")

	feature := Feature{}
	err := json.Unmarshal([]byte(featureJson), &feature)
	if err != nil {
		t.Errorf("couldn't unmarshal feature JSON")
	}
	check(t, Info, FeatureForceFromRule,
		LogData{"id": "test-feature", "rule": JSONLog{feature.Rules[0]}},
		"[INFO] Force value from rule (id = test-feature): {\"condition\":{\"browser\":\"firefox\",\"country\":{\"$in\":[\"US\",\"CA\"]}},\"force\":1}")

	override := map[string]interface{}{
		"a": 1,
		"b": "def",
		"c": []interface{}{1, 2, "ghi"},
		"d": map[string]interface{}{"e": "ghi", "f": 3.14},
	}
	check(t, Info, FeatureGlobalOverride,
		LogData{"id": "test-feature", "override": JSONLog{override}},
		"[INFO] Global override (id = test-feature, override = {\"a\":1,\"b\":\"def\",\"c\":[1,2,\"ghi\"],\"d\":{\"e\":\"ghi\",\"f\":3.14}})")

	value := 123
	check(t, Info, FeatureUseDefaultValue,
		LogData{"id": "test-feature", "value": JSONLog{value}},
		"[INFO] Use default value (id = test-feature): 123")

	check(t, Warn, SSEWaitingToReconnect,
		LogData{"key": "repo-key", "delay": 5 * time.Minute},
		"[WARN] Waiting to reconnect SSE stream: repo-key (delaying 5m0s)")

	check(t, Error, SSEError,
		LogData{"key": "repo-key", "error": errors.New("this is an error")},
		"[ERROR] SSE error (repo-key): this is an error")
}
