package condition

import (
	"strconv"
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
