package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strings"
)

// specs/019-scheduling-audit-and-gantt-integrity.md:35: independent scheduling
// regressions use small generic inputs, never source portfolio data.
var _ = Describe("Scheduling integrity audit", func() {
	var sp SchedulingParams
	params := Params{HorizonWeeks: 26}
	teams := []Team{{Name: "Atlas", Tracks: 2}}
	item := func(name string, rank int, weeks float64) Initiative {
		return Initiative{Name: name, StatedPriority: rank, Work: map[string]TeamWork{"Atlas": podWork(weeks)}}
	}
	BeforeEach(func() {
		sp = SchedulingParams{PeriodStart: specPeriodStart, EstimateModel: EstimateEffort, WipModel: WipOff, BufferPct: pctOf(0)}
	})

	It("keeps a legal future pin after a freeze in earlier weeks", func() {
		in := item("Pinned", 1, 4)
		in.PinnedStarts = map[string]int{"Atlas": 10}
		sp.Calendars = []CalendarWindow{{Kind: CalChangeFreeze, Scope: "Atlas", From: weekDate(0), To: weekDate(2), Effect: EffectBlockStart}}
		sl := sliceAt(scheduledFor(ComputeSchedule(teams, []Initiative{in}, params, sp), "Pinned"), "Atlas")
		Expect(sl.StartWeek).To(Equal(10))
		Expect(sl.FinishWeek).To(Equal(12))
	})
	It("rechecks blocked starts after waiting for occupied lanes", func() {
		sp.SplitTaxWeeks = 1
		sp.Calendars = []CalendarWindow{{Kind: CalChangeFreeze, Scope: "Atlas", From: weekDate(1), To: weekDate(2), Effect: EffectBlockStart}}
		s := ComputeSchedule(teams, []Initiative{item("Early", 1, 2), item("Later", 2, 4)}, params, sp)
		Expect(scheduledFor(s, "Later").StartWeek).To(Equal(3))
		Expect(scheduledFor(s, "Later").RawFinishWeek).To(Equal(5))
	})
	It("does not pay a large splitting tax when contiguous work finishes sooner", func() {
		sp.SplitTaxWeeks = 10
		s := ComputeSchedule(teams, []Initiative{item("Early", 1, 2), item("Later", 2, 4)}, params, sp)
		sl := sliceAt(scheduledFor(s, "Later"), "Atlas")
		Expect(sl.StartWeek).To(Equal(1))
		Expect(sl.FinishWeek).To(Equal(3))
		Expect(sl.Phases).To(BeEmpty())
	})
	It("charges growth overhead after a capacity gap", func() {
		sp.SplitTaxWeeks = 1
		sp.Calendars = []CalendarWindow{{Kind: CalEvent, Scope: "Atlas", From: weekDate(1), To: weekDate(2), Effect: EffectReduceCapacity}}
		s := ComputeSchedule(teams, []Initiative{item("Early", 1, 1), item("Growing", 2, 6)}, params, sp)
		sl := sliceAt(scheduledFor(s, "Growing"), "Atlas")
		if len(sl.Phases) == 0 {
			Expect(sl.LanesUsed * (sl.FinishWeek - sl.StartWeek)).To(BeNumerically(">=", 6))
			return
		}
		productive := 0
		ramp := 0
		previous := 0
		for _, ph := range sl.Phases {
			if ph.Lanes > previous {
				ramp += sp.SplitTaxWeeks
			}
			for w := ph.FromWeek; w < ph.ToWeek; w++ {
				if ramp > 0 {
					ramp--
				} else {
					productive += ph.Lanes
				}
			}
			previous = ph.Lanes
		}
		Expect(productive).To(BeNumerically(">=", 6))
	})
	DescribeTable("does not allocate a fictitious lead", func(value string) {
		sp.LeadCapacity = map[string]int{"pm": 1}
		a, b := item("First", 1, 1), item("Second", 2, 1)
		a.Leads = map[string]string{"pm": value}
		b.Leads = map[string]string{"pm": value}
		s := ComputeSchedule(teams, []Initiative{a, b}, params, sp)
		Expect(scheduledFor(s, "First").StartWeek).To(Equal(0))
		Expect(scheduledFor(s, "Second").StartWeek).To(Equal(0))
		Expect(scheduledFor(s, "First").Assumptions).NotTo(BeEmpty())
	}, Entry("unassigned", " TBD "), Entry("not applicable", "n/a"), Entry("none", "None"), Entry("not required", "Not Required"))
	It("shares capacity for case and whitespace variants of the same named lead", func() {
		sp.LeadCapacity = map[string]int{"pm": 1}
		a, b := item("First", 1, 1), item("Second", 2, 1)
		a.Leads = map[string]string{"pm": "Lead One"}
		b.Leads = map[string]string{"pm": "  lead   one "}
		s := ComputeSchedule(teams, []Initiative{a, b}, params, sp)
		Expect(scheduledFor(s, "Second").StartWeek).To(BeNumerically(">=", scheduledFor(s, "First").RawFinishWeek))
	})
	It("terminates a permanently closed lead gate with an explicit hold", func() {
		sp.LeadCapacity = map[string]int{"pm": 0}
		a := item("Held", 1, 1)
		a.Leads = map[string]string{"pm": "Lead One"}
		done := make(chan *Schedule, 1)
		go func() { done <- ComputeSchedule(teams, []Initiative{a}, params, sp) }()
		var s *Schedule
		Eventually(done, "1s").Should(Receive(&s))
		held := scheduledFor(s, "Held")
		Expect(held.Slices).To(BeEmpty())
		Expect(held.BindingConstraint).To(Equal("lead"))
		Expect(strings.Join(held.Assumptions, " ")).To(ContainSubstring("0"))
	})
	It("rejects ambiguous names rather than copying the first row's schedule", func() {
		inputs := []Initiative{item("Alpha", 1, 1), item(" alpha ", 2, 20)}
		Expect(ValidateInitiativeNames(inputs)).To(HaveOccurred())
		s := ComputeSchedule(teams, inputs, params, sp)
		Expect(s.Assumptions).NotTo(BeEmpty())
		Expect(s.Initiatives).To(HaveLen(2))
		for _, si := range s.Initiatives {
			Expect(si.Slices).To(BeEmpty())
		}
	})
	It("qualifies unresolved and self team dependencies", func() {
		a := item("Partial", 1, 2)
		a.Work["Atlas"] = podWork(2, "Missing", "Atlas")
		si := scheduledFor(ComputeSchedule(teams, []Initiative{a}, params, sp), "Partial")
		Expect(si.Provisional).To(BeTrue())
		Expect(strings.Join(si.Assumptions, " ")).To(ContainSubstring("Missing"))
	})
	It("does not release a successor of an unschedulable predecessor", func() {
		a := Initiative{Name: "Predecessor", StatedPriority: 1, Work: map[string]TeamWork{"Missing": podWork(5)}}
		b := item("Successor", 2, 1)
		b.AfterInitiatives = []string{"Predecessor"}
		s := ComputeSchedule(teams, []Initiative{a, b}, params, sp)
		Expect(scheduledFor(s, "Predecessor").Verdict).To(Equal("unschedulable"))
		Expect(scheduledFor(s, "Successor").Slices).To(BeEmpty())
	})
	It("propagates provisional dependency evidence", func() {
		a := item("Predecessor", 1, 2)
		a.AfterInitiatives = []string{"Missing"}
		b := item("Successor", 2, 1)
		b.AfterInitiatives = []string{"Predecessor"}
		s := ComputeSchedule(teams, []Initiative{a, b}, params, sp)
		Expect(scheduledFor(s, "Predecessor").Provisional).To(BeTrue())
		Expect(scheduledFor(s, "Successor").Provisional).To(BeTrue())
	})
	It("does not improve an order by omitting its dated commitment", func() {
		sp.EstimateModel = EstimateWallClock
		sp.AcceptedOrdering = "engine"
		long, short := item("Long", 2, 26), item("Short", 1, 1)
		short.TargetDate = specPeriodStart
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 1}}, []Initiative{long, short}, params, sp)
		Expect(scheduledFor(s, "Short").Slices).To(HaveLen(1))
		Expect(scheduledFor(s, "Short").WeeksLate).To(Equal(1))
		Expect(s.ObjectiveScore).To(BeNumerically(">", 0))
		Expect(s.UnscheduledWeight).To(Equal(0.0))
	})
})
