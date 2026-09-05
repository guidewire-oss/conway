package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strings"
)

// specs/018-scheduling-capacity-and-timeline-correctness.md:40: measure
// delivered work and occupied capacity, rather than only checking dates.
var _ = Describe("Scheduling capacity accounting", func() {
	var sp SchedulingParams
	var params Params
	work := func(name string, priority int, effort float64) Initiative {
		return Initiative{Name: name, StatedPriority: priority, Work: map[string]TeamWork{"Atlas": podWork(effort)}}
	}
	delivered := func(sl *WorkSlice, loss float64) float64 {
		laneWeeks := sl.LanesUsed * (sl.FinishWeek - sl.StartWeek)
		if len(sl.Phases) > 0 {
			Expect(sl.Phases).To(HaveLen(1), "this accounting helper covers flat or single-phase work")
			laneWeeks = 0
			for _, ph := range sl.Phases {
				laneWeeks += ph.Lanes * (ph.ToWeek - ph.FromWeek)
			}
			laneWeeks -= sl.Phases[0].Lanes * sp.SplitTaxWeeks
		}
		return float64(laneWeeks) * (1 - loss)
	}
	BeforeEach(func() {
		sp = SchedulingParams{PeriodStart: specPeriodStart, EstimateModel: EstimateEffort, WipModel: WipOff, BufferPct: pctOf(0), SplitTaxWeeks: 1}
		params = Params{HorizonWeeks: 26, CapacityLoss: .1}
	})

	It("places first and later full-width estimates within an enforceable drum target", func() {
		sp.TargetUtilization = .9
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 4}}, []Initiative{work("First", 1, 18), work("Second", 2, 18)}, params, sp)
		for _, name := range []string{"First", "Second"} {
			si := scheduledFor(s, name)
			Expect(si.Slices).To(HaveLen(1))
			sl := sliceAt(si, "Atlas")
			Expect(sl.StartWeek).To(BeNumerically("<", 26))
			Expect(sl.LanesUsed).To(BeNumerically("<=", 3))
			Expect(delivered(sl, .1)).To(BeNumerically(">=", 18))
		}
		for _, w := range podScheduleFor(s, "Atlas").Weeks {
			Expect(w.Busy).To(BeNumerically("<=", 3))
		}
	})

	It("qualifies a sub-lane target and allows successive one-lane work", func() {
		sp.TargetUtilization = .9
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 1}}, []Initiative{work("First", 1, 4), work("Second", 2, 4)}, params, sp)
		Expect(scheduledFor(s, "First").Slices).To(HaveLen(1))
		Expect(scheduledFor(s, "Second").Slices).To(HaveLen(1))
		Expect(strings.Join(s.Assumptions, " ")).To(ContainSubstring("below one whole lane"))
	})

	DescribeTable("conserves effort after selecting fewer lanes", func(effort float64, chunk, expectedDuration int) {
		sp.SplitMinWeeks = chunk
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 5}}, []Initiative{work("First", 1, effort)}, params, sp)
		sl := sliceAt(scheduledFor(s, "First"), "Atlas")
		Expect(sl.LanesUsed).To(Equal(1))
		Expect(sl.FinishWeek - sl.StartWeek).To(Equal(expectedDuration))
		Expect(delivered(sl, .1)).To(BeNumerically(">=", effort))
	}, Entry("below chunk threshold", 18.0, 20, 20), Entry("small estimate with capacity loss", 1.0, 0, 2))

	It("retains roster teams whose assignments are held and teams with no assignments", func() {
		i := work("Held", 1, 4)
		i.KitPct = 0
		sp.KitGate = 1
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 2}, {Name: "Beacon", Tracks: 3}}, []Initiative{i}, params, sp)
		Expect(scheduledFor(s, "Held").Slices).To(BeEmpty())
		for _, name := range []string{"Atlas", "Beacon"} {
			ps := podScheduleFor(s, name)
			Expect(ps.Slices).To(BeEmpty())
			Expect(ps.Tracks).To(BeNumerically(">", 0))
			Expect(ps.Weeks).NotTo(BeEmpty())
		}
	})

	It("does not grow through a future reservation and overbook physical lanes", func() {
		params.CapacityLoss = 0
		early := work("Early", 1, 1)
		reserved := work("Reserved", 2, 4)
		reserved.PinnedStarts = map[string]int{"Atlas": 4}
		later := work("Later", 3, 18)
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 2}}, []Initiative{early, reserved, later}, params, sp)
		for _, name := range []string{"Early", "Reserved", "Later"} {
			Expect(scheduledFor(s, name).Slices).To(HaveLen(1))
		}
		Expect(sliceAt(scheduledFor(s, "Reserved"), "Atlas").StartWeek).To(Equal(4))
		for _, w := range podScheduleFor(s, "Atlas").Weeks {
			Expect(w.Busy).To(BeNumerically("<=", w.Tracks), "week %d", w.Week)
		}
		Expect(delivered(sliceAt(scheduledFor(s, "Later"), "Atlas"), 0)).To(BeNumerically(">=", 18))
	})

	It("preserves running capacity and lets unrelated teams bypass the drum rope", func() {
		sp.TargetUtilization = .9
		sp.WipModel = WipDrumGated
		sp.MaxConcurrentInitiatives = 1
		running := work("Running", 1, 18)
		running.InFlight = true
		unrelated := Initiative{Name: "Independent", StatedPriority: 3, Work: map[string]TeamWork{"Beacon": podWork(1)}}
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 4}, {Name: "Beacon", Tracks: 4}}, []Initiative{running, work("New", 2, 12), unrelated}, params, sp)
		Expect(sliceAt(scheduledFor(s, "Running"), "Atlas").LanesUsed).To(Equal(4))
		Expect(sliceAt(scheduledFor(s, "New"), "Atlas").LanesUsed).To(BeNumerically("<=", 3))
		Expect(scheduledFor(s, "Independent").StartWeek).To(Equal(0))
		for _, w := range podScheduleFor(s, "Atlas").Weeks {
			Expect(w.Busy).To(BeNumerically("<=", w.Tracks))
		}
	})

	It("respects a calendar closure and resumes within the drum lane budget", func() {
		sp.TargetUtilization = .75
		sp.Calendars = []CalendarWindow{{Kind: CalEvent, Scope: "Atlas", From: weekDate(0), To: weekDate(2), Effect: EffectReduceCapacity}}
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 4}}, []Initiative{work("First", 1, 12)}, params, sp)
		sl := sliceAt(scheduledFor(s, "First"), "Atlas")
		Expect(sl.StartWeek).To(Equal(3))
		Expect(sl.LanesUsed).To(Equal(3))
		Expect(sl.FinishWeek - sl.StartWeek).To(Equal(5))
		for _, w := range podScheduleFor(s, "Atlas").Weeks {
			Expect(w.Busy).To(BeNumerically("<=", 3))
		}
	})

	It("does not charge splitting overhead for work that can use only one lane", func() {
		s := ComputeSchedule([]Team{{Name: "Atlas", Tracks: 1}}, []Initiative{work("First", 1, 5), work("Second", 2, 5)}, params, sp)
		for _, name := range []string{"First", "Second"} {
			sl := sliceAt(scheduledFor(s, name), "Atlas")
			Expect(sl.FinishWeek - sl.StartWeek).To(Equal(6))
			Expect(sl.LanesUsed).To(Equal(1))
		}
	})

	It("closes split phases across every week of a non-working gap", func() {
		params.CapacityLoss = 0
		sp.Calendars = []CalendarWindow{{Kind: CalEvent, Scope: "Atlas", From: weekDate(2), To: weekDate(3), Effect: EffectReduceCapacity}}
		// Exercise the split candidate directly: the public scheduler may now
		// prefer cheaper contiguous placement, which does not carry phases.
		phases, _ := splitPlace(&podCalendar{tracks: 2, busy: []int{1}}, 2, 0, 18, 0, 1, compileCalendars(parseCalendars(sp, 26)), "Atlas", "", false, 2, 0, "Growing", 2)
		Expect(phases).NotTo(BeEmpty())
		for _, ph := range phases {
			Expect(ph.FromWeek >= 4 || ph.ToWeek <= 2).To(BeTrue(), "phase %+v overlaps a non-working week", ph)
		}
	})

	DescribeTable("accounts for each growth ramp without including a completion hold", func(tax, workFinish int) {
		params.CapacityLoss = 0
		sp.SplitTaxWeeks = tax
		sp.Calendars = []CalendarWindow{{Kind: CalChangeFreeze, Scope: "Atlas", From: weekDate(3), To: weekDate(8), Effect: EffectBlockFinish}}
		phases, _ := splitPlace(&podCalendar{tracks: 2, busy: []int{1}}, 2, 0, 4, 0, tax, compileCalendars(parseCalendars(sp, 26)), "Atlas", "", false, 2, 0, "Growing", 2)
		Expect(phases).NotTo(BeEmpty())
		// Two split events each charge the configured tax, including growth
		// during an existing ramp. Four effort weeks then take two weeks.
		Expect(phases[len(phases)-1].ToWeek).To(Equal(workFinish))
	}, Entry("one-week ramps", 1, 4), Entry("growth during a two-week ramp", 2, 6))
})
