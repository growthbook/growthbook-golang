package condition

import (
	"strconv"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
)

func benchInCondEval(b *testing.B, n int) {
	elems := make([]any, n)
	for i := range elems {
		elems[i] = "id-" + strconv.Itoa(i)
	}
	c := NewInCond(value.Arr(elems...))
	hit := value.New("id-" + strconv.Itoa(n-1))
	miss := value.New("missing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Eval(hit, nil)
		_ = c.Eval(miss, nil)
	}
}

func benchIniCondEval(b *testing.B, n int) {
	elems := make([]any, n)
	for i := range elems {
		elems[i] = "ID-" + strconv.Itoa(i)
	}
	c := NewIniCond(value.Arr(elems...))
	hit := value.New("id-" + strconv.Itoa(n-1))
	miss := value.New("missing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Eval(hit, nil)
		_ = c.Eval(miss, nil)
	}
}

func BenchmarkInCondEval10(b *testing.B)    { benchInCondEval(b, 10) }
func BenchmarkInCondEval100(b *testing.B)   { benchInCondEval(b, 100) }
func BenchmarkInCondEval1000(b *testing.B)  { benchInCondEval(b, 1000) }
func BenchmarkInCondEval10000(b *testing.B) { benchInCondEval(b, 10000) }

func BenchmarkIniCondEval10(b *testing.B)    { benchIniCondEval(b, 10) }
func BenchmarkIniCondEval100(b *testing.B)   { benchIniCondEval(b, 100) }
func BenchmarkIniCondEval1000(b *testing.B)  { benchIniCondEval(b, 1000) }
func BenchmarkIniCondEval10000(b *testing.B) { benchIniCondEval(b, 10000) }
