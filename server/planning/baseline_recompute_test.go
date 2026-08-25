package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BaselineInputs.RecomputeWith", func() {
	var in BaselineInputs

	BeforeEach(func() {
		teams, inits := Demo()
		in = NewBaselineInputs(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, DemoScheduling())
	})

	It("can rebuild without the WIP-model comparison", func() {
		Expect(in.RecomputeWith(ScheduleOptions{}).WipModels).To(BeEmpty())
	})

	// AC 7.1: a baseline reproduces its stored schedule exactly. Recompute is what
	// the baseline endpoints call, so its answer must not narrow.
	It("leaves Recompute reproducing the stored schedule exactly", func() {
		Expect(in.Recompute().WipModels).NotTo(BeEmpty())
		Expect(in.Recompute()).To(Equal(in.RecomputeWith(ScheduleOptions{CompareWipModels: true})))
	})
})
