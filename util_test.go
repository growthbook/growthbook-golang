package growthbook

import (
	"math"
	"strconv"
	"testing"

	. "github.com/franela/goblin"
)

func TestUtils(t *testing.T) {
	g := Goblin(t)
	g.Describe("utils", func() {
		g.It("bucket ranges", func() {
			// Normal 50/50 split
			g.Assert(round(getBucketRanges(2, 1, []float64{}))).
				Equal(varRanges(0, 0.5, 0.5, 1))

			// Reduced coverage
			g.Assert(round(getBucketRanges(2, 0.5, []float64{}))).
				Equal(varRanges(0, 0.25, 0.5, 0.75))

			// Zero coverage
			g.Assert(round(getBucketRanges(2, 0, []float64{}))).
				Equal(varRanges(0, 0, 0.5, 0.5))

			// More variations
			g.Assert(round(getBucketRanges(4, 1, []float64{}))).
				Equal(varRanges(0, 0.25, 0.25, 0.5, 0.5, 0.75, 0.75, 1))

			// Uneven weights
			g.Assert(round(getBucketRanges(2, 1, []float64{0.4, 0.6}))).
				Equal(varRanges(0, 0.4, 0.4, 1))

			// Uneven weights, more variations
			g.Assert(round(getBucketRanges(3, 1, []float64{0.2, 0.3, 0.5}))).
				Equal(varRanges(0, 0.2, 0.2, 0.5, 0.5, 1))

			// Uneven weights, more variations, reduced coverage
			g.Assert(round(getBucketRanges(3, 0.2, []float64{0.2, 0.3, 0.5}))).
				Equal(varRanges(0, 0.2*0.2, 0.2, 0.2+0.3*0.2, 0.5, 0.5+0.5*0.2))
		})

		g.It("choose variation", func() {
			evenRange := varRanges(0, 0.5, 0.5, 1)
			reducedRange := varRanges(0, 0.25, 0.5, 0.75)
			zeroRange := varRanges(0, 0.5, 0.5, 0.5, 0.5, 1)

			g.Assert(chooseVariation(0.2, evenRange)).Equal(0)
			g.Assert(chooseVariation(0.6, evenRange)).Equal(1)
			g.Assert(chooseVariation(0.4, evenRange)).Equal(0)
			g.Assert(chooseVariation(0.8, evenRange)).Equal(1)
			g.Assert(chooseVariation(0, evenRange)).Equal(0)
			g.Assert(chooseVariation(0.5, evenRange)).Equal(1)

			g.Assert(chooseVariation(0.2, reducedRange)).Equal(0)
			g.Assert(chooseVariation(0.6, reducedRange)).Equal(1)
			g.Assert(chooseVariation(0.4, reducedRange)).Equal(-1)
			g.Assert(chooseVariation(0.8, reducedRange)).Equal(-1)

			g.Assert(chooseVariation(0.5, zeroRange)).Equal(2)
		})

		g.It("persists assignment when coverage changes", func() {
			g.Assert(round(getBucketRanges(2, 0.1, []float64{0.4, 0.6}))).
				Equal(varRanges(0, 0.4*0.1, 0.4, 0.4+0.6*0.1))

			g.Assert(round(getBucketRanges(2, 1, []float64{0.4, 0.6}))).
				Equal(varRanges(0, 0.4, 0.4, 1))
		})

		g.It("handles weird experiment values", func() {
			g.Assert(round(getBucketRanges(2, -0.2, []float64{}))).
				Equal(varRanges(0, 0, 0.5, 0.5))

			g.Assert(round(getBucketRanges(2, 1.5, []float64{}))).
				Equal(varRanges(0, 0.5, 0.5, 1))

			g.Assert(round(getBucketRanges(2, 1, []float64{0.4, 0.1}))).
				Equal(varRanges(0, 0.5, 0.5, 1))

			g.Assert(round(getBucketRanges(2, 1, []float64{0.7, 0.6}))).
				Equal(varRanges(0, 0.5, 0.5, 1))

			g.Assert(round(getBucketRanges(4, 1, []float64{0.4, 0.4, 0.2}))).
				Equal(varRanges(0, 0.25, 0.25, 0.5, 0.5, 0.75, 0.75, 1))
		})

		g.It("querystring force invalid url", func() {
			g.Assert(getQueryStringOverride("my-test", "", 10)).
				IsNil()

			g.Assert(getQueryStringOverride("my-test", "http://example.com", 10)).
				IsNil()

			g.Assert(getQueryStringOverride("my-test", "http://example.com?", 10)).
				IsNil()

			g.Assert(getQueryStringOverride("my-test", "http://example.com?somequery", 10)).
				IsNil()

			g.Assert(getQueryStringOverride("my-test", "http://example.com??&&&?#", 10)).
				IsNil()
		})

		g.It("calculates namespace inclusion correctly", func() {
			included := 0
			for i := 0; i < 10000; i++ {
				if inNamespace(strconv.Itoa(i), &Namespace{"namespace1", 0, 0.4}) {
					included++
				}
			}
			g.Assert(included).Equal(4042)

			included = 0
			for i := 0; i < 10000; i++ {
				if inNamespace(strconv.Itoa(i), &Namespace{"namespace1", 0.4, 1}) {
					included++
				}
			}
			g.Assert(included).Equal(5958)

			included = 0
			for i := 0; i < 10000; i++ {
				if inNamespace(strconv.Itoa(i), &Namespace{"namespace2", 0, 0.4}) {
					included++
				}
			}
			g.Assert(included).Equal(3984)
		})
	})
}

// Helper to create VariationRange arrays for comparison.
func varRanges(values ...float64) []VariationRange {
	result := []VariationRange{}
	for i := range values {
		if i%2 == 0 {
			result = append(result, VariationRange{values[i], values[i+1]})
		}
	}
	return result
}

// Helper to round variation ranges for comparison with fixed test
// values.
func round(ranges []VariationRange) []VariationRange {
	result := []VariationRange{}
	for i := range ranges {
		rmin := math.Round(ranges[i].Min*100) / 100
		rmax := math.Round(ranges[i].Max*100) / 100
		result = append(result, VariationRange{rmin, rmax})
	}
	return result
}
