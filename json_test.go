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
		content, err := ioutil.ReadFile("cases/fnv.json")
		if err != nil {
			log.Fatal(err)
		}

		cases := []interface{}{}
		err = json.Unmarshal(content, &cases)
		if err != nil {
			log.Fatal(err)
		}

		for icase := range cases {
			c, ok := cases[icase].([]interface{})
			if !ok {
				log.Fatal(err)
			}
			s, oks := c[0].(string)
			if !oks {
				log.Fatal(err)
			}
			v, okn := c[1].(float64)
			if !okn {
				log.Fatal(err)
			}
			n := uint32(v)
			g.It(fmt.Sprintf("fnv.json[%d] %s", n, s), func() {
				g.Assert(hashFnv32a(s) % 1000).Equal(n)
			})
		}
	})
}
