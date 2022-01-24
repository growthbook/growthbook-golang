package growthbook

import (
	"testing"

	. "github.com/franela/goblin"
)

func TestSubscriptions(t *testing.T) {
	g := Goblin(t)

	g.Describe("subscriptions", func() {
		var context *Context
		var gb *GrowthBook
		var exp1 *Experiment
		var exp2 *Experiment

		g.BeforeEach(func() {
			context = NewContext().WithAttributes(Attributes{"id": "1"})
			gb = New(context)
			exp1 = NewExperiment("experiment-1").WithVariations("result1", "result2")
			exp2 = NewExperiment("experiment-2").WithVariations("result3", "result4")
		})

		g.It("can subscribe to experiments", func() {
			var savedExp *Experiment
			called := 0
			gb.Subscribe(func(exp *Experiment, result *ExperimentResult) {
				savedExp = exp
				called++
			})

			gb.Run(exp1)
			gb.Run(exp1)

			g.Assert(savedExp).Equal(exp1)
			// Subscription only gets triggered once for repeated experiment
			// runs.
			g.Assert(called).Equal(1)

			savedExp = nil
			called = 0

			gb.ClearSavedResults()
			gb.Run(exp1)
			// Change attributes to change experiment result so subscription
			// gets triggered twice.
			gb.WithAttributes(Attributes{"id": "3"})
			gb.Run(exp1)

			g.Assert(savedExp).Equal(exp1)
			g.Assert(called).Equal(2)
		})

		g.It("can unsubscribe from experiments", func() {
			var savedExp *Experiment
			called := 0
			unsubscribe := gb.Subscribe(func(exp *Experiment, result *ExperimentResult) {
				savedExp = exp
				called++
			})

			gb.Run(exp1)
			gb.WithAttributes(Attributes{"id": "3"})
			unsubscribe()
			gb.Run(exp1)

			g.Assert(savedExp).Equal(exp1)
			g.Assert(called).Equal(1)
		})

		g.It("can track experiment results", func() {
			called := 0
			context.WithTrackingCallback(func(exp *Experiment, result *ExperimentResult) {
				called++
			})

			gb.Run(exp1)
			gb.Run(exp2)
			gb.Run(exp1)
			gb.Run(exp2)
			gb.WithAttributes(Attributes{"id": "3"})
			gb.Run(exp1)
			gb.Run(exp2)
			gb.Run(exp1)
			gb.Run(exp2)
			g.Assert(called).Equal(4)
		})

		g.It("can retrieve all experiment results", func() {
			gb.Run(exp1)
			gb.Run(exp2)
			g.Assert(len(gb.GetAllResults())).Equal(2)
		})
	})
}
