package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the demo dataset", func() {
	It("ships a rich dependency network and at least one constraint", func() {
		teams, inits := Demo()
		plan := &Plan{Initiatives: inits}
		for _, tm := range teams {
			plan.Teams = append(plan.Teams, tm.Name)
		}
		net := BuildNetwork(plan)
		Expect(len(net.Edges)).To(BeNumerically(">=", 5),
			"the demo should show a rich dependency network")
		loads := Utilization(plan, teams, Params{HorizonWeeks: 26, CapacityLoss: 0.10})
		constraints := 0
		for _, l := range loads {
			if l.Constraint {
				constraints++
			}
		}
		Expect(constraints).NotTo(BeZero(), "the demo should surface at least one over-capacity pod")
	})
})
