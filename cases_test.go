package growthbook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

type cases struct {
	EvalCondition          []JsonTuple[evalConditionCase]          `json:"evalCondition"`
	ChooseVariation        []JsonTuple[chooseVariationCase]        `json:"chooseVariation"`
	Hash                   []JsonTuple[hashCase]                   `json:"hash"`
	Run                    []JsonTuple[runCase]                    `json:"run"`
	Feature                []JsonTuple[featureCase]                `json:"feature"`
	GetBucketRange         []JsonTuple[getBucketRangeCase]         `json:"getBucketRange"`
	GetQueryStringOverride []JsonTuple[getQueryStringOverrideCase] `json:"getQueryStringOverride"`
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

	t.Run("evalCondition", func(t *testing.T) {
		for _, tuple := range cases.EvalCondition {
			tuple.val.test(t)
		}
	})

	t.Run("chooseVariation", func(t *testing.T) {
		for _, tuple := range cases.ChooseVariation {
			tuple.val.test(t)
		}
	})

	t.Run("hash", func(t *testing.T) {
		for _, tuple := range cases.Hash {
			tuple.val.test(t)
		}
	})

	t.Run("run", func(t *testing.T) {
		for _, tuple := range cases.Run {
			tuple.val.test(t)
		}
	})

	t.Run("feature", func(t *testing.T) {
		for _, tuple := range cases.Feature {
			tuple.val.test(t)
		}
	})

	t.Run("getBucketRange", func(t *testing.T) {
		for _, tuple := range cases.GetBucketRange {
			tuple.val.test(t)
		}
	})

	t.Run("getQueryStringOverride", func(t *testing.T) {
		for _, tuple := range cases.GetQueryStringOverride {
			tuple.val.test(t)
		}
	})
}

func (c *evalConditionCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		attrs := value.Obj(c.Attrs)
		require.Equal(t, c.Res, c.Cond.Eval(attrs, c.Groups))
	})
}

func (c *chooseVariationCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		require.Equal(t, c.Expected, chooseVariation(c.N, c.Ranges))
	})
}

func (c *hashCase) test(t *testing.T) {
	name := fmt.Sprintf(`hash("%s","%s","%d")`, c.Seed, c.Value, c.Version)
	t.Run(name, func(t *testing.T) {
		require.Equal(t, c.Expected, hash(c.Seed, c.Value, c.Version))
	})
}

func (c *runCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		client, err := c.Env.client()
		require.Nil(t, err)

		res := client.RunExperiment(context.TODO(), c.Exp)
		require.Equal(t, c.Value, res.Value)
		require.Equal(t, c.InExperiment, res.InExperiment)
		require.Equal(t, c.HashUsed, res.HashUsed)
	})
}

func (c *featureCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		client, err := c.Env.client()
		require.Nil(t, err)

		res := client.EvalFeature(context.TODO(), c.FeatureName)
		require.Equal(t, c.Expected, res)
	})
}

func (c *getBucketRangeCase) test(t *testing.T) {
	t.Run(c.Name, func(t *testing.T) {
		client, err := NewClient(context.TODO())
		require.Nil(t, err)

		i := c.Inputs.val
		res := client.getBucketRanges(i.Num, i.Coverage, i.Weights)
		require.Equal(t, c.Expected, roundRanges(res))
	})
}

func (c *getQueryStringOverrideCase) test(t *testing.T) {
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
