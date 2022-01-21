package growthbook

import (
	"testing"

	. "github.com/franela/goblin"
)

func TestFeatures(t *testing.T) {
	g := Goblin(t)
	g.Describe("features", func() {
		g.It("can set features", func() {
			context := NewContext().
				WithAttributes(Attributes{
					"id": "123",
				})
			growthbook := New(context).
				WithFeatures(FeatureMap{
					"feature": &Feature{
						DefaultValue: 0,
					},
				})
			g.Assert(growthbook.Feature("feature")).Equal(&FeatureResult{
				Value:  0,
				On:     false,
				Off:    true,
				Source: DefaultValueResultSource,
			})
		})

		g.It("updates attributes with setAttributes", func() {
			context := NewContext().
				WithAttributes(Attributes{
					"foo": 1,
					"bar": 2,
				})

			growthbook := New(context)
			growthbook = growthbook.WithAttributes(Attributes{"foo": 2, "baz": 3})

			g.Assert(context.Attributes).Equal(Attributes{
				"foo": 2,
				"baz": 3,
			})
		})
	})
}
