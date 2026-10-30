package growthbook

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every section of the shared spec (cases.json) is either run by a test or
// excluded here with a reason, so a new section in a spec update can't be
// skipped without anyone noticing. TestCasesJson decodes the corpus into the
// cases struct, which silently ignores a section it has no field for. Nested
// sections are listed as `parent.child`.

// Sections the tests run, and the test that runs each one
var specRunSections = map[string]string{
	"evalCondition":                        "TestCasesJson",
	"chooseVariation":                      "TestCasesJson",
	"hash":                                 "TestCasesJson",
	"run":                                  "TestCasesJson",
	"feature":                              "TestCasesJson",
	"getBucketRange":                       "TestCasesJson",
	"getQueryStringOverride":               "TestCasesJson",
	"inNamespace":                          "TestCasesJson",
	"getEqualWeights":                      "TestCasesJson",
	"decrypt":                              "TestCasesJson",
	"stickyBucket":                         "TestCasesJson",
	"contextualBandit":                     "TestCasesJson",
	"pruneCondition":                       "TestCasesJson",
	"reducePayload":                        "TestCasesJson",
	"savedGroupReferencesV2.evalCondition": "TestCasesJson",
	"savedGroupReferencesV2.feature":       "TestCasesJson",
	"savedGroupReferencesV2.run":           "TestCasesJson",
	"savedGroupReferencesRemote.evalCondition": "TestCasesJson",
	"savedGroupReferencesRemote.feature":       "TestCasesJson",
}

// Sections the tests don't run, and why
var specExcludedSections = map[string]string{
	"urlRedirect": "URL redirect experiments are a browser feature",
}

func specSections(t *testing.T) []string {
	data, err := os.ReadFile("cases.json")
	require.NoError(t, err)
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))
	var sections []string
	for name, raw := range top {
		switch strings.TrimSpace(string(raw))[0] {
		case '[':
			sections = append(sections, name)
		case '{':
			var nested map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &nested))
			for child := range nested {
				sections = append(sections, name+"."+child)
			}
		}
		// Anything else is specVersion
	}
	sort.Strings(sections)
	return sections
}

// decodedSections lists the sections the cases struct has a field for
func decodedSections(typ reflect.Type, prefix string, out map[string]bool) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			continue
		}
		if field.Type.Kind() == reflect.Struct {
			decodedSections(field.Type, prefix+name+".", out)
		} else {
			out[prefix+name] = true
		}
	}
}

func TestSpecCoverageEverySectionIsRunOrExcluded(t *testing.T) {
	var unaccounted []string
	for _, section := range specSections(t) {
		_, run := specRunSections[section]
		_, excluded := specExcludedSections[section]
		if !run && !excluded {
			unaccounted = append(unaccounted, section)
		}
	}
	require.Empty(t, unaccounted, "Run these spec sections or exclude them with a reason")
}

func TestSpecCoverageListedSectionsExistInTheSpec(t *testing.T) {
	inSpec := map[string]bool{}
	for _, section := range specSections(t) {
		inSpec[section] = true
	}
	var unknown []string
	for _, listed := range []map[string]string{specRunSections, specExcludedSections} {
		for section := range listed {
			if !inSpec[section] {
				unknown = append(unknown, section)
			}
		}
	}
	require.Empty(t, unknown, "These sections aren't in cases.json")
}

func TestSpecCoverageEveryRunSectionIsDecoded(t *testing.T) {
	decoded := map[string]bool{}
	decodedSections(reflect.TypeOf(cases{}), "", decoded)
	for section := range specRunSections {
		require.True(t, decoded[section], "The cases struct has no field for %s", section)
	}
}

func TestSpecCoverageNoSectionIsBothRunAndExcluded(t *testing.T) {
	for section := range specExcludedSections {
		_, run := specRunSections[section]
		require.False(t, run, section)
	}
}

func TestSpecCoverageEveryExcludedSectionHasAReason(t *testing.T) {
	for section, reason := range specExcludedSections {
		require.NotEmpty(t, strings.TrimSpace(reason), section)
	}
}
