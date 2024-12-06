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
	Attrs        map[string]any
	Exp          *Experiment
	Value        FeatureValue
	InExperiment bool
	HashUsed     bool
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
		attrs := value.New(c.Attrs)
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
		attrs, ok := c.Attrs["attributes"].(map[string]any)
		require.True(t, ok)
		client, err := NewClient(context.TODO(),
			WithAttributes(attrs),
			WithLogger(debugLogger()),
		)
		require.Nil(t, err)
		res := client.RunExperiment(context.TODO(), c.Exp)
		assert.Equal(t, c.Value, res.Value)
		assert.Equal(t, c.InExperiment, res.InExperiment)
		assert.Equal(t, c.HashUsed, res.HashUsed)
	})
}
