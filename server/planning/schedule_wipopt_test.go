package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The WIP-model comparison costs three extra full schedules (D22 as amended), and
// it is only ever read by the scheduling-assumptions form — a panel the planner
// opens. Computing it on every /schedule made the endpoint four times more
// expensive than the answer it returns.
//
// Measured on the dev cluster 2026-08-25, plan gCZ_G3LgQFWvHjV8 (35 teams, 29
// initiatives, loaded to 100% of raw track capacity): POST /schedule took 6-8s
// warm and 43s cold.
var _ = Describe("ComputeScheduleWith", func() {
	var teams []Team
	var inits []Initiative
	var params Params
	var sp SchedulingParams

	BeforeEach(func() {
		teams, inits = Demo()
		params = Params{HorizonWeeks: 26, CapacityLoss: 0.1}
		sp = DemoScheduling()
	})

	It("omits the comparison when it was not asked for", func() {
		got := ComputeScheduleWith(teams, inits, params, sp, ScheduleOptions{})
		Expect(got.WipModels).To(BeEmpty(), "three extra schedules must not be computed unasked")
	})

	It("includes the comparison when it was asked for", func() {
		got := ComputeScheduleWith(teams, inits, params, sp, ScheduleOptions{CompareWipModels: true})
		Expect(got.WipModels).NotTo(BeEmpty())
	})

	// AC 1.4: same inputs, identical output field for field. Skipping the
	// comparison must change nothing else, or the Order view would depend on
	// whether a panel happened to be open.
	It("changes nothing but the comparison", func() {
		with := ComputeScheduleWith(teams, inits, params, sp, ScheduleOptions{CompareWipModels: true})
		without := ComputeScheduleWith(teams, inits, params, sp, ScheduleOptions{})

		Expect(without.WipModels).To(BeEmpty())
		without.WipModels = with.WipModels // the only field that may differ
		Expect(without).To(Equal(with))
	})

	// ComputeSchedule is exported, and BaselineInputs.Recompute calls it; AC 7.1
	// requires a baseline to reproduce its stored schedule exactly, so its
	// behaviour must not move.
	It("leaves ComputeSchedule comparing, as every existing caller expects", func() {
		Expect(ComputeSchedule(teams, inits, params, sp).WipModels).NotTo(BeEmpty())
		Expect(ComputeSchedule(teams, inits, params, sp)).To(Equal(
			ComputeScheduleWith(teams, inits, params, sp, ScheduleOptions{CompareWipModels: true})))
	})
})
