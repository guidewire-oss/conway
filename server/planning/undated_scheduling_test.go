package planning

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// specs/020-undated-capacity-scheduling.md:39: missing dates must not prevent placement.
var _ = Describe("Undated capacity scheduling", func() {
	var teams []Team
	var inits []Initiative
	var sp SchedulingParams
	params := Params{HorizonWeeks: 6}
	BeforeEach(func() {
		teams = []Team{{Name: "Atlas", Tracks: 1}, {Name: "Beacon", Tracks: 1}, {Name: "Cedar", Tracks: 1}}
		inits = nil
		for n, team := range teams {
			inits = append(inits, Initiative{Name: team.Name + " work", StatedPriority: n + 1,
				Leads: map[string]string{"pm": "Shared Owner"}, Work: map[string]TeamWork{team.Name: podWork(8)}})
		}
		sp = SchedulingParams{WipModel: WipOff, EstimateModel: EstimateEffort, BufferPct: pctOf(0)}
	})
	mode := func(value string) {
		Expect(json.Unmarshal([]byte(`{"leadCapacityMode":"`+value+`"}`), &sp)).To(Succeed())
	}
	It("fills independent free tracks without input dates or implicit lead holds", func() {
		s := ComputeSchedule(teams, inits, params, sp)
		for _, i := range s.Initiatives {
			Expect(i.StartWeek).To(Equal(0))
			Expect(i.RawFinishWeek).To(Equal(8))
			Expect(i.Slices).To(HaveLen(1))
			Expect(i.WeeksLate).To(BeZero())
			Expect(strings.Join(i.Assumptions, " ")).To(ContainSubstring("advisory"))
		}
	})
	It("retains explicit hard enforcement of default thresholds", func() {
		mode("hard")
		s := ComputeSchedule(teams, inits, params, sp)
		Expect(scheduledFor(s, "Cedar work").BindingConstraint).To(Equal("lead"))
		Expect(scheduledFor(s, "Cedar work").Slices).To(BeEmpty())
	})
	It("preserves legacy explicit role limits including zero", func() {
		sp.LeadCapacity = map[string]int{"pm": 0}
		s := ComputeSchedule(teams, inits, params, sp)
		for _, i := range s.Initiatives {
			Expect(i.Slices).To(BeEmpty())
			Expect(i.BindingConstraint).To(Equal("lead"))
		}
	})
	It("can explicitly make configured thresholds advisory including zero", func() {
		sp.LeadCapacity = map[string]int{"pm": 0}
		mode("advisory")
		s := ComputeSchedule(teams, inits, params, sp)
		for _, i := range s.Initiatives {
			Expect(i.Slices).To(HaveLen(1))
			Expect(i.BindingConstraint).NotTo(Equal("lead"))
			Expect(strings.Join(i.Assumptions, " ")).To(ContainSubstring("advisory"))
		}
	})
	It("fills a contiguous gap before a later pinned reservation", func() {
		inits = []Initiative{
			{Name: "Reserved", StatedPriority: 1, Work: map[string]TeamWork{"Atlas": podWork(2)}, PinnedStarts: map[string]int{"Atlas": 4}},
			{Name: "Undated", StatedPriority: 2, Work: map[string]TeamWork{"Atlas": podWork(3)}},
		}
		s := ComputeSchedule(teams, inits, params, sp)
		u := scheduledFor(s, "Undated")
		Expect(u.StartWeek).To(Equal(0))
		Expect(u.RawFinishWeek).To(Equal(3))
		Expect(u.Slices[0].Phases).To(BeEmpty())
		Expect(scheduledFor(s, "Reserved").StartWeek).To(Equal(4))
	})
	It("enforces dependencies and calendar starts even when leads are advisory", func() {
		sp.PeriodStart = specPeriodStart
		sp.Calendars = []CalendarWindow{{Kind: CalChangeFreeze, Scope: "Atlas", From: weekDate(0), To: weekDate(1), Effect: EffectBlockStart}}
		inits[0].Work = map[string]TeamWork{"Atlas": podWork(1)}
		inits[1].Work = map[string]TeamWork{"Beacon": podWork(1)}
		inits[1].AfterInitiatives = []string{inits[0].Name}
		s := ComputeSchedule(teams, inits, params, sp)
		a, b := scheduledFor(s, inits[0].Name), scheduledFor(s, inits[1].Name)
		Expect(a.StartWeek).To(Equal(2))
		Expect(b.StartWeek).To(BeNumerically(">=", a.CommitWeek))
	})
	It("does not warn about a lead threshold when work does not overlap", func() {
		sp.LeadCapacity = map[string]int{"pm": 1}
		mode("advisory")
		for n := range inits {
			inits[n].Work = map[string]TeamWork{"Atlas": podWork(1)}
		}
		s := ComputeSchedule(teams, inits, params, sp)
		for _, i := range s.Initiatives {
			Expect(strings.Join(i.Assumptions, " ")).NotTo(ContainSubstring("advisory"))
		}
	})
})
