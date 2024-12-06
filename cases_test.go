package growthbook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cases struct {
	EvalCondition   []JsonTuple[evalConditionCase]   `json:"evalCondition"`
	ChooseVariation []JsonTuple[chooseVariationCase] `json:"chooseVariation"`
	Hash            []JsonTuple[hashCase]            `json:"hash"`
	Run             []JsonTuple[runCase]             `json:"run"`
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
		attrs := c.Env.Attributes
		client, err := NewClient(context.TODO(),
			WithAttributes(attrs),
			WithForcedVariations(c.Env.ForcedVariations),
			WithFeatures(c.Env.Features),
			WithSavedGroups(c.Env.SavedGroups),
			WithLogger(debugLogger()),
		)
		require.Nil(t, err)

		if enabled := c.Env.Enabled; enabled != nil {
			client, err = client.WithEnabled(*enabled)
			require.Nil(t, err)
		}

		if qaMode := c.Env.QaMode; qaMode != nil {
			client, err = client.WithQaMode(*qaMode)
			require.Nil(t, err)
		}

		if url := c.Env.Url; url != "" {
			client, err = client.WithUrl(url)
			require.Nil(t, err)
		}

		res := client.RunExperiment(context.TODO(), c.Exp)
		assert.Equal(t, c.Value, res.Value)
		assert.Equal(t, c.InExperiment, res.InExperiment)
		assert.Equal(t, c.HashUsed, res.HashUsed)
	})
}
