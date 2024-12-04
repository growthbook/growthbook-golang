package growthbook

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

type cases struct {
	EvalCondition []evalConditionCase `json:"evalCondition"`
}

type evalConditionCase struct {
	name   string
	cond   json.RawMessage
	attrs  map[string]any
	res    bool
	groups condition.SavedGroups
}

func (c *evalConditionCase) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	if err := json.Unmarshal(fields[0], &c.name); err != nil {
		return err
	}
	if err := json.Unmarshal(fields[1], &c.cond); err != nil {
		return err
	}
	if err := json.Unmarshal(fields[2], &c.attrs); err != nil {
		return err
	}
	if err := json.Unmarshal(fields[3], &c.res); err != nil {
		return err
	}

	if len(fields) == 4 {
		return nil
	}

	if err := json.Unmarshal(fields[4], &c.groups); err != nil {
		return err
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
		for _, c := range cases.EvalCondition {
			var cond condition.Base
			err := json.Unmarshal(c.cond, &cond)
			require.Nil(t, err, c.name)
			attrs := value.New(c.attrs)
			require.Equal(t, c.res, cond.Eval(attrs, c.groups), c.name)
		}
	})
}
