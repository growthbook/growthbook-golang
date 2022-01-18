package growthbook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	. "github.com/franela/goblin"
)

func TestJSON(t *testing.T) {
	g := Goblin(t)
	g.Describe("json test suite", func() {
		fnvContent, err := ioutil.ReadFile("cases/fnv.json")
		if err != nil {
			log.Fatal(err)
		}

		fnvCases := []interface{}{}
		err = json.Unmarshal(fnvContent, &fnvCases)
		if err != nil {
			log.Fatal(err)
		}

		for icase := range fnvCases {
			c, ok := fnvCases[icase].([]interface{})
			if !ok {
				log.Fatal("unpacking fvn test data")
			}
			string, ok0 := c[0].(string)
			value, ok1 := c[1].(float64)
			if !ok0 || !ok1 {
				log.Fatal("unpacking fvn test data")
			}
			ivalue := uint32(value)
			g.It(fmt.Sprintf("fnv.json[%d] %s", icase, string), func() {
				g.Assert(hashFnv32a(string) % 1000).Equal(ivalue)
			})
		}

		conditionsContent, err := ioutil.ReadFile("cases/conditions.json")
		if err != nil {
			log.Fatal(err)
		}

		conditionsCases := []interface{}{}
		err = json.Unmarshal(conditionsContent, &conditionsCases)
		if err != nil {
			log.Fatal(err)
		}

		for icase := range conditionsCases {
			c, ok := conditionsCases[icase].([]interface{})
			if !ok {
				log.Fatal("unpacking conditions test data")
			}
			name, ok0 := c[0].(string)
			condition, ok1 := c[1].(map[string]interface{})
			value, ok2 := c[2].(map[string]interface{})
			expected, ok3 := c[3].(bool)
			if !ok0 || !ok1 || !ok2 || !ok3 {
				log.Fatal("unpacking conditions test data")
			}

			g.It(fmt.Sprintf("conditions.json[%d] %s", icase, name), func() {
				g.Assert(value).IsNotNil()
				g.Assert(expected).IsNotNil()
				cond, err := BuildCondition(condition)
				// fmt.Printf("CONDITION: %#v\n", cond)
				attrs := Attributes(value)
				// fmt.Printf("ATTRIBUTES: %#v\n", attrs)
				g.Assert(err).IsNil()
				g.Assert(cond.Eval(&attrs)).Equal(expected)
			})
			// if icase > 2 {
			// 	break
			// }
		}
	})
}
