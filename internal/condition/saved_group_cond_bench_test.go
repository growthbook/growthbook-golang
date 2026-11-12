package condition

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// BenchmarkSavedGroupEval measures evaluation without payload parsing, including
// deep chains and siblings that must independently visit the same group.
func BenchmarkSavedGroupEval(b *testing.B) {
	for _, tc := range []struct {
		name     string
		depth    int
		siblings bool
	}{
		{"single", 1, false},
		{"nested", 5, false},
		{"deep", 150, false},
		{"siblings", 4, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			groups := SavedGroups{
				"leaf": savedGroup{cond: savedGroupListCond{
					path:       []string{"id"},
					membership: NewInCond(value.Arr("u1")),
				}},
			}
			var cond Condition = savedGroupCond{id: "leaf"}
			if tc.siblings {
				cond = AndConds{cond, cond, cond}
			}
			for i := 1; i < tc.depth; i++ {
				id := strconv.Itoa(i)
				groups[id] = savedGroup{cond: cond}
				cond = savedGroupCond{id: id}
			}
			base := Base{cond: cond}
			attrs := value.ObjValue{"id": value.Str("u1")}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !base.Eval(attrs, groups) {
					b.Fatal("expected saved-group match")
				}
			}
		})
	}
}

// BenchmarkInGroupStrings checks one hit and one miss against a large group of
// strings, in both entry shapes.
func BenchmarkInGroupStrings(b *testing.B) {
	values := make([]string, 10000)
	for i := range values {
		values[i] = strconv.Quote("id-" + strconv.Itoa(i))
	}
	list := "[" + strings.Join(values, ",") + "]"
	for _, tc := range []struct{ name, entry string }{
		{"legacy", list},
		{"list", `{"type":"list","attributeKey":"id","values":` + list + `}`},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var groups SavedGroups
			if err := json.Unmarshal([]byte(`{"g":`+tc.entry+`}`), &groups); err != nil {
				b.Fatal(err)
			}
			cond := NewInGroupCond("g")
			hit, miss := value.Str("id-9999"), value.Str("missing")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = cond.Eval(hit, groups, nil)
				_ = cond.Eval(miss, groups, nil)
			}
		})
	}
}
