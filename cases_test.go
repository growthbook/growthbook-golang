package growthbook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

type cases struct {
	EvalCondition          JsonTuples[evalConditionCase]          `json:"evalCondition"`
	ChooseVariation        JsonTuples[chooseVariationCase]        `json:"chooseVariation"`
	Hash                   JsonTuples[hashCase]                   `json:"hash"`
	Run                    JsonTuples[runCase]                    `json:"run"`
	Feature                JsonTuples[featureCase]                `json:"feature"`
	GetBucketRange         JsonTuples[getBucketRangeCase]         `json:"getBucketRange"`
	GetQueryStringOverride JsonTuples[getQueryStringOverrideCase] `json:"getQueryStringOverride"`
	InNamespace            JsonTuples[inNamespaceCase]            `json:"inNamespace"`
	GetEqualWeights        JsonTuples[getEqualWeightsCase]        `json:"getEqualWeights"`
	Decrypt                JsonTuples[decryptCase]                `json:"decrypt"`
	StickyBucket           JsonTuples[stickyBucketTestCase]       `json:"stickyBucket"`
}

type evalConditionCase struct {
	Name   string
	Cond   condition.Base
	Attrs  map[string]any
	Res    bool
	Groups condition.SavedGroups
}

type chooseVariationCase struct {
	Name     string
	N        float64
	Ranges   []BucketRange
	Expected int
}

type hashCase struct {
	Seed     string
	Value    string
	Version  int
	Expected *float64
}

type runCase struct {
	Name         string
	Env          env
	Exp          *Experiment
	Value        FeatureValue
	InExperiment bool
	HashUsed     bool
}

type featureCase struct {
	Name        string
	Env         env
	FeatureName string
	Expected    *FeatureResult
}

type getBucketRangeCase struct {
	Name   string
	Inputs JsonTuple[struct {
		Num      int
		Coverage float64
		Weights  []float64
	}]
	Expected []BucketRange
}

type getQueryStringOverrideCase struct {
	Name          string
	Key           string
	Url           string
	NumVariations int
	Expected      *int
}

type inNamespaceCase struct {
	Name      string
	Id        string
	Namespace *Namespace
	Expected  bool
}

type getEqualWeightsCase struct {
	NumVariations int
	Expected      []float64
}

type decryptCase struct {
	Name      string
	Encrypted string
	Key       string
	Expected  string
}

type stickyBucketTestCase struct {
	Name                string
	Env                 env
	ExistingAssignments []StickyBucketAssignmentDoc
	FeatureName         string
	Expected            *ExperimentResult
	ExpectedAssignments map[string]*StickyBucketAssignmentDoc
}

type env struct {
	Attributes       Attributes            `json:"attributes"`
	Features         FeatureMap            `json:"features"`
	Enabled          *bool                 `json:"enabled"`
	Url              string                `json:"url"`
	ForcedVariations ForcedVariationsMap   `json:"forcedVariations"`
	QaMode           *bool                 `json:"qaMode"`
	SavedGroups      condition.SavedGroups `json:"savedGroups"`
}

type JsonTuple[T any] struct {
	val T
}

func (t *JsonTuple[T]) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	val := reflect.ValueOf(&t.val).Elem()
	valType := val.Type()
	for i, elemText := range fields {
		err := json.Unmarshal(elemText, val.Field(i).Addr().Interface())
		if err != nil {
			return fmt.Errorf("Failed to unmarshal %v field from %s case: %w", valType.Field(i).Name, fields[0], err)
		}
	}
	return nil
}

type JsonTuples[T JsonCase] []JsonTuple[T]
type JsonCase interface{ test(t *testing.T) }

func (ts JsonTuples[T]) run(name string, t *testing.T) {
	t.Run(name, func(t *testing.T) {
		for _, tuple := range ts {
			tuple.val.test(t)
		}
	})
}

func TestCasesJson(t *testing.T) {
	file := "cases.json"
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var cases cases
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}

	cases.EvalCondition.run("evalCondition", t)
	cases.ChooseVariation.run("chooseVariation", t)
	cases.Hash.run("hash", t)
	cases.Run.run("run", t)
	cases.Feature.run("feature", t)
	cases.GetBucketRange.run("getBucketRange", t)
	cases.GetQueryStringOverride.run("getQueryStringOverride", t)
	cases.InNamespace.run("inNamespace", t)
	cases.GetEqualWeights.run("getEqualWeights", t)
	cases.Decrypt.run("decrypt", t)
	cases.StickyBucket.run("stickyBucket", t)
}

func (c evalConditionCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		attrs := value.Obj(c.Attrs)
		require.Equal(t, c.Res, c.Cond.Eval(attrs, c.Groups))
	})
}

func (c chooseVariationCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		require.Equal(t, c.Expected, chooseVariation(c.N, c.Ranges))
	})
}

func (c hashCase) test(t *testing.T) {
	name := fmt.Sprintf(`hash("%s","%s","%d")`, c.Seed, c.Value, c.Version)
	t.Run(name, func(t *testing.T) {
		require.Equal(t, c.Expected, hash(c.Seed, c.Value, c.Version))
	})
}

func (c runCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		client, err := c.Env.client()
		require.Nil(t, err)

		res := client.RunExperiment(context.TODO(), c.Exp)
		require.Equal(t, c.Value, res.Value)
		require.Equal(t, c.InExperiment, res.InExperiment)
		require.Equal(t, c.HashUsed, res.HashUsed)
	})
}

func (c featureCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		client, err := c.Env.client()
		require.Nil(t, err)

		res := client.EvalFeature(context.TODO(), c.FeatureName)
		require.Equal(t, c.Expected, res)
	})
}

func (c getBucketRangeCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		client, err := NewClient(context.TODO())
		require.Nil(t, err)

		i := c.Inputs.val
		res := client.getBucketRanges(i.Num, i.Coverage, i.Weights)
		require.Equal(t, c.Expected, roundRanges(res))
	})
}

func (c getQueryStringOverrideCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		url, err := url.Parse(c.Url)
		require.Nil(t, err)
		res, ok := getQueryStringOverride(c.Key, url, c.NumVariations)
		if c.Expected == nil {
			require.False(t, ok)
		} else {
			require.True(t, ok)
			require.Equal(t, *c.Expected, res)
		}
	})
}

func (c inNamespaceCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		res := c.Namespace.inNamespace(c.Id)
		require.Equal(t, c.Expected, res)
	})
}

func (c getEqualWeightsCase) test(t *testing.T) {
	name := fmt.Sprintf(`("%v")`, c.NumVariations)
	t.Run(name, func(t *testing.T) {
		res := getEqualWeights(c.NumVariations)
		require.Equal(t, roundArr(c.Expected), roundArr(res))
	})
}

func (c decryptCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		res, err := decrypt(c.Encrypted, c.Key)
		if c.Expected != "" {
			require.Nil(t, err)
			require.Equal(t, c.Expected, res)
		} else {
			require.NotNil(t, err)
			require.Equal(t, "", res)
		}
	})
}

func (c stickyBucketTestCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		// Create a sticky bucket service with pre-populated assignments
		service := NewInMemoryStickyBucketService()
		for _, assignment := range c.ExistingAssignments {
			err := service.SaveAssignments(&assignment)
			require.NoError(t, err)
		}

		// Create the client with the environment and sticky bucket service
		client, err := c.Env.client()
		require.Nil(t, err)

		// Add the sticky bucket service to the client
		client.stickyBucketService = service

		// Add space for cached assignments
		client.stickyBucketAssignments = make(StickyBucketAssignments)

		// Set up a debug logger
		client.logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))

		// Evaluate the feature
		featureResult := client.EvalFeature(context.TODO(), c.FeatureName)

		// Check the experiment result
		if c.Expected == nil {
			require.Nil(t, featureResult.ExperimentResult, "Expected no experiment result, but got one")
		} else if featureResult.ExperimentResult == nil {
			require.NotNil(t, c.Expected, "Expected an experiment result, but got none")
		} else {
			// Compare the important fields
			result := featureResult.ExperimentResult
			require.Equal(t, c.Expected.Value, result.Value, "Value mismatch")
			require.Equal(t, c.Expected.VariationId, result.VariationId, "VariationId mismatch")
			require.Equal(t, c.Expected.InExperiment, result.InExperiment, "InExperiment mismatch")
			require.Equal(t, c.Expected.Key, result.Key, "Key mismatch")
			require.Equal(t, c.Expected.HashAttribute, result.HashAttribute, "HashAttribute mismatch")
			require.Equal(t, c.Expected.StickyBucketUsed, result.StickyBucketUsed, "StickyBucketUsed mismatch")
		}

		// Check if assignments match expectations
		if c.ExpectedAssignments != nil {
			for key, expectedDoc := range c.ExpectedAssignments {
				parts := parseAttributeKey(key)
				if len(parts) == 2 {
					attributeName, attributeValue := parts[0], parts[1]

					// Get the actual document from the service
					actualDoc, err := service.GetAssignments(attributeName, attributeValue)
					require.NoError(t, err)
					require.NotNil(t, actualDoc, "Expected a document for %s, but none was found", key)

					// Check assignments match
					for expKey, expVal := range expectedDoc.Assignments {
						actualVal, exists := actualDoc.Assignments[expKey]
						require.True(t, exists, "Assignment %s not found in document for %s", expKey, key)
						require.Equal(t, expVal, actualVal, "Assignment value mismatch for %s in document %s", expKey, key)
					}
				}
			}
		}
	})
}

// Helper function to parse an attribute key (format: "attributeName||attributeValue")
func parseAttributeKey(key string) []string {
	for i := 0; i < len(key)-1; i++ {
		if key[i] == '|' && key[i+1] == '|' {
			return []string{key[:i], key[i+2:]}
		}
	}
	return []string{}
}

func (e *env) client() (*Client, error) {
	client, err := NewClient(context.TODO(),
		WithAttributes(e.Attributes),
		WithForcedVariations(e.ForcedVariations),
		WithFeatures(e.Features),
		WithSavedGroups(e.SavedGroups),
	)
	if err != nil {
		return nil, err
	}

	if enabled := e.Enabled; enabled != nil {
		client, err = client.WithEnabled(*enabled)
		if err != nil {
			return nil, err
		}
	}

	if qaMode := e.QaMode; qaMode != nil {
		client, err = client.WithQaMode(*qaMode)
		if err != nil {
			return nil, err
		}
	}

	if url := e.Url; url != "" {
		client, err = client.WithUrl(url)
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}
