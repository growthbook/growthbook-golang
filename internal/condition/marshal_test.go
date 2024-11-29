package condition

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

var (
	age10   = NewFieldCond("age", NewValueCond(10))
	nameBob = NewFieldCond("name", NewValueCond("Bob"))
)

func TestEmptyBase(t *testing.T) {
	var b Base
	require.True(t, b.Eval(value.Null(), nil))
}

func TestLogicMarshaling(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		result Condition
	}{
		{"empty", `{}`,
			AndConds{}},
		{"$and", `{"$and": [{"age": 10}, {"name": "Bob"}]}`,
			AndConds{age10, nameBob}},
		{"$or", `{"$or": [{"age": 10}, {"name": "Bob"}]}`,
			OrConds{age10, nameBob}},
		{"$nor", `{"$nor": [{"age": 10}, {"name": "Bob"}]}`,
			NorConds{age10, nameBob}},
		{"$not", `{"$not": {"age": 10}}`,
			NotCond{age10}},
		{"nested", `{"$not": {"$and": [{"age": 10}, {"name": "Bob"}]}}`,
			NotCond{AndConds{age10, nameBob}}},
		{"multiple", `{"$and": [{"age": 10}], "$or": [{"name": "Bob"}]}`,
			AndConds{AndConds{age10}, OrConds{nameBob}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var b Base
			err := json.Unmarshal([]byte(test.json), &b)
			require.Nil(t, err)
			require.Equal(t, test.result, b.cond)
		})
	}
}

func TestValueMarshaling(t *testing.T) {
	tests := map[string]Condition{
		`10`:           NewValueCond(10),
		`{"$eq": 10}`:  NewCompCond(eqOp, 10),
		`{"$ne": 10}`:  NewCompCond(neOp, 10),
		`{"$lt": 10}`:  NewCompCond(ltOp, 10),
		`{"$gt": 10}`:  NewCompCond(gtOp, 10),
		`{"$gte": 10}`: NewCompCond(gteOp, 10),
		`{"$lte": 10}`: NewCompCond(lteOp, 10),

		`{"$veq": 1}`:  NewVersionCond(veqOp, 1),
		`{"$vne": 1}`:  NewVersionCond(vneOp, 1),
		`{"$vgte": 1}`: NewVersionCond(vgteOp, 1),
		`{"$vlt": 1}`:  NewVersionCond(vltOp, 1),
		`{"$vlte": 1}`: NewVersionCond(vlteOp, 1),

		`{"$in": ["tag1", "tag2"]}`:  NewInCond(value.Arr("tag1", "tag2")),
		`{"$nin": ["tag1", "tag2"]}`: NewNotInCond(value.Arr("tag1", "tag2")),

		`{"$inGroup": "admins"}`:    NewInGroupCond("admins"),
		`{"$notInGroup": "admins"}`: NewNotInGroupCond("admins"),

		`{"$regex": "foo"}`:           NewRegexCond(regexp.MustCompile("foo")),
		`{"$size": 10}`:               NewSizeCond(10),
		`{"$elemMatch": {"age": 10}}`: NewElemMatchCond(age10),
		`{"$elemMatch": {"$eq": 10}}`: NewElemMatchCond(NewCompCond(eqOp, 10)),
		`{"$all": [10, {"$eq": 10}]}`: AllConds{NewValueCond(10), NewCompCond(eqOp, 10)},
		`{"$type": "string"}`:         NewTypeCond("string"),
		`{"$exists": true}`:           NewExistsCond(true),
	}
	for s, result := range tests {
		t.Run(s, func(t *testing.T) {
			j := fmt.Sprintf(`{"field": %s}`, s)
			var b Base
			err := json.Unmarshal([]byte(j), &b)
			require.Nil(t, err)
			f := b.cond.(FieldCond)
			require.Equal(t, result, f.cond)
		})
	}

}
