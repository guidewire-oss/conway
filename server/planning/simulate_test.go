package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the flow simulator (what-if levers)", func() {
	rho := func(r SimResult, pod string) float64 {
		for _, l := range r.Loads {
			if l.Team == pod {
				return l.Rho
			}
		}
		return -1
	}

	It("improves flow when capacity is added to the constraint and WIP reduced", func() {
		teams, inits := Demo()
		params := Params{HorizonWeeks: 26, CapacityLoss: 0.10}
		before, after := Simulate(teams, inits, params, []Lever{
			{Type: "addCapacity", Pod: "Delta", N: 3},
			{Type: "reduceWip", N: 0.15},
		})
		Expect(before.Constraints).NotTo(BeZero(), "the demo baseline should have a constraint")
		Expect(after.Constraints).To(BeNumerically("<=", before.Constraints),
			"levers must not add constraints")
		Expect(rho(after, "Delta")).To(BeNumerically("<", rho(before, "Delta")),
			"Delta's load should fall after adding capacity")
		Expect(after.Fitting).To(BeNumerically(">=", before.Fitting),
			"more initiatives should fit (or at least not fewer)")
		Expect(after.MedianLeadWeeks).To(BeNumerically("<=", before.MedianLeadWeeks),
			"the median lead should not increase")
	})

	It("drops a deferred initiative from the simulated total", func() {
		teams, inits := Demo()
		_, after := Simulate(teams, inits, Params{}, []Lever{{Type: "defer", Initiative: "Telemetry GA"}})
		Expect(after.Total).To(Equal(len(inits)-1), "defer should drop one initiative")
	})

	It("reassigns a pod's work and drops a pod from an initiative", func() {
		teams, inits := Demo()
		params := Params{HorizonWeeks: 26, CapacityLoss: 0.10}
		// reassign Delta's work to Beacon (7 tracks) -> Delta rho drops
		_, after := Simulate(teams, inits, params, []Lever{{Type: "reassign", Pod: "Delta", ToPod: "Beacon"}})
		Expect(rho(after, "Delta")).To(BeNumerically("<=", 0.01),
			"reassign should empty Delta's demand")
		// drop a pod from one initiative
		beforeDrop := ComputeResult(teams, inits, params, 0)
		_, afterDrop := Simulate(teams, inits, params, []Lever{{Type: "dropPod", Pod: "Delta", Initiative: "Telemetry GA"}})
		Expect(afterDrop.Loads).NotTo(BeNil())
		Expect(rho(afterDrop, "Delta")).To(BeNumerically("<", rho(beforeDrop, "Delta")),
			"dropPod should reduce Delta's load")
	})
})
