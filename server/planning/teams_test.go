package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the roster parser", func() {
	It("parses a teams CSV into pod rows with derived tracks", func() {
		csv := "Pod Name,Developers,Location,Pairs\n" +
			"Alpha,\"a@x,b@x,c@x,d@x,e@x,f@x\",Bengaluru,no\n" + // 6 devs, no pairing -> 6 tracks
			"Gamma,\"g@x,h@x,i@x,j@x\",Krakow,yes\n" + // 4 devs, pairs -> 2 tracks
			"Empty,,Remote,no\n" // 0 devs
		teams, err := ParseTeamsCSV([]byte(csv))
		Expect(err).NotTo(HaveOccurred())
		Expect(teams).To(HaveLen(3))
		Expect(teams[0].Devs).To(Equal(6))
		Expect(teams[0].Pairs).To(BeFalse())
		Expect(teams[0].EffectiveTracks()).To(Equal(6))
		Expect(teams[1].Devs).To(Equal(4))
		Expect(teams[1].Pairs).To(BeTrue())
		Expect(teams[1].EffectiveTracks()).To(Equal(2), "4 devs pairing run 2 tracks")
		Expect(teams[1].Site).To(Equal("Krakow"))
	})

	It("derives tracks: pairing halves, an explicit override wins", func() {
		Expect((Team{Devs: 7, Pairs: true}).EffectiveTracks()).To(Equal(4), "ceil(7/2)")
		Expect((Team{Devs: 7, Tracks: 2}).EffectiveTracks()).To(Equal(2), "override wins")
	})

	// Spec 014 AC 2.1/2.2: the roster may carry a per-pod capacity loss as a
	// percent ("15", "15%") or a fraction ("0.15"); an empty cell inherits the
	// plan global; an out-of-range cell refuses the roster, naming the pod.
	It("reads the loss column in percent or fraction form, empty inherits", func() {
		csv := "Pod Name,Developers,Capacity Loss %\n" +
			"Alpha,3,15\n" +
			"Beta,2,15%\n" +
			"Gamma,2,0.15\n" +
			"Delta,2,\n"
		teams, err := ParseTeamsCSV([]byte(csv))
		Expect(err).NotTo(HaveOccurred())
		want := map[string]float64{"Alpha": 0.15, "Beta": 0.15, "Gamma": 0.15, "Delta": 0}
		for _, tm := range teams {
			Expect(tm.CapacityLoss).To(Equal(want[tm.Name]), "%s loss", tm.Name)
		}
	})

	It("refuses out-of-range loss cells, naming the pod", func() {
		for _, cell := range []string{"150%", "-3", "100%", "abc"} {
			csv := "Pod Name,Developers,Capacity Loss %\nAlpha,3," + cell + "\n"
			_, err := ParseTeamsCSV([]byte(csv))
			Expect(err).To(HaveOccurred(), "cell %q must refuse the roster", cell)
			Expect(err.Error()).To(ContainSubstring("Alpha"), "the refusal names the pod")
		}
	})
})

var _ = Describe("Utilization", func() {
	It("computes per-pod demand against capacity, hottest first", func() {
		// Alpha: 6 tracks; Gamma: 2 tracks. Horizon 26, loss 10% -> cap factor 23.4/track.
		teams := []Team{{Name: "Alpha", Devs: 6}, {Name: "Gamma", Devs: 4, Pairs: true}}
		plan := &Plan{Initiatives: []Initiative{{Work: map[string]TeamWork{
			"Alpha": {Weeks: 20, InPath: true},
			"Gamma": {Weeks: 60, InPath: true}, // way over its 2-track capacity
		}}}}
		loads := Utilization(plan, teams, Params{HorizonWeeks: 26, CapacityLoss: 0.10})
		byName := map[string]PodLoad{}
		for _, l := range loads {
			byName[l.Team] = l
		}
		aj := byName["Alpha"]
		Expect(aj.Tracks).To(Equal(6))
		Expect(aj.CapacityWeeks).To(BeNumerically("~", 6*26*0.9, 1e-9))
		Expect(aj.Rho).To(BeNumerically("<", 1))
		Expect(aj.Constraint).To(BeFalse(), "Alpha should be under capacity")
		co := byName["Gamma"]
		Expect(co.Constraint).To(BeTrue(), "Gamma should be the constraint")
		Expect(co.Rho).To(BeNumerically(">", 1))
		Expect(loads[0].Team).To(Equal("Gamma"), "the constraint sorts first")
	})

	It("flags demand with zero capacity as the InfiniteRho sentinel", func() {
		plan := &Plan{Initiatives: []Initiative{{Work: map[string]TeamWork{"Ghost": {Weeks: 5, InPath: true}}}}}
		loads := Utilization(plan, nil, Params{}) // Ghost has demand but no roster entry
		// InfiniteRho (not literal +Inf) — encoding/json can't marshal Inf/NaN, so a
		// plan with any zero-capacity, demand-carrying pod would silently return an
		// empty response body otherwise.
		Expect(loads).To(HaveLen(1))
		Expect(loads[0].Constraint).To(BeTrue())
		Expect(loads[0].Rho).To(Equal(InfiniteRho))
	})
})

var _ = Describe("EffectiveLoss (spec 014)", func() {
	It("uses the pod override when set, else the plan global", func() {
		Expect((Team{CapacityLoss: 0.3}).EffectiveLoss(0.1)).To(Equal(0.3))
		Expect((Team{}).EffectiveLoss(0.1)).To(Equal(0.1))
	})
})
