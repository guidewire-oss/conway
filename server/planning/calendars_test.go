package planning

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// FR-018: calendar constraints — org-wide change-freeze windows, per-site
// non-working windows, and capacity that arrives part-way through the period.
//
// The fixture is small enough to check by hand: one pod, one initiative, one
// window under test. Dates map to weeks off the fixture's period start.
var _ = Describe("calendar windows", func() {
	var (
		teams []Team
		inits []Initiative
		sp    SchedulingParams
	)

	weekDate := func(w int) string {
		t0, _ := time.Parse(isoDate, specPeriodStart)
		return t0.AddDate(0, 0, w*7).Format(isoDate)
	}

	BeforeEach(func() {
		teams = []Team{{Name: "Delta", Devs: 4, Tracks: 2}}
		inits = []Initiative{
			{Name: "Dated", Work: map[string]TeamWork{"Delta": podWork(4)},
				StatedPriority: 1, TargetDate: weekDate(6)},
		}
		sp = SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25)}
	})

	sched := func() *Schedule {
		return ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
	}
	siOf := func(s *Schedule) ScheduledInitiative { return s.Initiatives[0] }

	It("leaves a window-free schedule exactly as it was", func() {
		// The guard: every spec below compares against this baseline, so it
		// must be the known-good shape — a 4-week slice committing at w5.
		si := siOf(sched())
		Expect(si.StartWeek).To(Equal(0))
		Expect(si.CommitWeek).To(Equal(5)) // 4w + 1w buffer
	})

	Describe("AC X.3: a freeze window covering the work", func() {
		It("moves the commit to the first week after the freeze and names the freeze", func() {
			// Freeze weeks 0-3 inclusive (to = weekDate(3) covers week 3):
			// the slice cannot start until the freeze lifts at week 4.
			sp.Calendars = []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(0), To: weekDate(3), Effect: EffectBlockStart},
			}
			si := siOf(sched())
			Expect(si.StartWeek).To(Equal(4), "the start waits until the freeze lifts (to is inclusive)")
			Expect(si.BindingConstraint).To(Equal(bindFreeze))
		})

		It("moves a commit landing inside a block-finish window past its end", func() {
			sp.Calendars = []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(2), To: weekDate(6), Effect: EffectBlockFinish},
			}
			si := siOf(sched())
			Expect(si.CommitWeek).To(BeNumerically(">", 6),
				"no completion inside the freeze; the finish is the first week after it")
			for _, sl := range si.Slices {
				Expect(sl.FinishWeek).To(BeNumerically(">", 6), "the slice finishes after the freeze lifts")
			}
		})
	})

	Describe("site non-working windows (reduce-capacity)", func() {
		BeforeEach(func() {
			teams = []Team{
				{Name: "Delta", Devs: 4, Tracks: 2, Site: "Kraków"},
			}
		})

		It("stretches work across a site's non-working weeks", func() {
			// Weeks 1-2 non-working at the site (to = weekDate(2) inclusive
			// covers weeks 1 and 2): capacity is zero there, so a 4-week
			// slice starting w0 finishes at w6, not w4. The stretch is
			// visible, not silent.
			sp.Calendars = []CalendarWindow{
				{Kind: CalSiteNonWorking, Scope: "Kraków", From: weekDate(1), To: weekDate(2), Effect: EffectReduceCapacity},
			}
			si := siOf(sched())
			Expect(si.StartWeek).To(Equal(0))
			Expect(si.RawFinishWeek).To(Equal(6), "two non-working weeks stretch a 4-week slice to six calendar weeks")
		})

		It("leaves pods at other sites untouched", func() {
			teams = append(teams, Team{Name: "Atlas", Devs: 4, Tracks: 2, Site: "Toronto"})
			sp.Calendars = []CalendarWindow{
				{Kind: CalSiteNonWorking, Scope: "Kraków", From: weekDate(1), To: weekDate(3), Effect: EffectReduceCapacity},
			}
			inits = append(inits, Initiative{Name: "Elsewhere",
				Work: map[string]TeamWork{"Atlas": podWork(4)}, StatedPriority: 2})
			s := sched()
			var away ScheduledInitiative
			for _, si := range s.Initiatives {
				if si.Name == "Elsewhere" {
					away = si
				}
			}
			Expect(away.StartWeek).To(Equal(0))
			Expect(away.RawFinishWeek).To(Equal(4), "Toronto works through Kraków's holiday")
		})
	})

	Describe("capacity arriving part-way through the period", func() {
		It("schedules work from the window's start when the pod has no capacity before it", func() {
			// An onboarding window modelled as zero capacity before week 4
			// (to = weekDate(3) inclusive covers weeks 0-3): the slice cannot
			// occupy those weeks, so it runs w4-w8.
			sp.Calendars = []CalendarWindow{
				{Kind: CalEvent, Scope: "Delta", From: weekDate(0), To: weekDate(3), Effect: EffectReduceCapacity},
			}
			si := siOf(sched())
			Expect(si.StartWeek).To(Equal(4), "no tracks exist before the window lifts")
			Expect(si.RawFinishWeek).To(Equal(8))
		})
	})

	Describe("the window model itself", func() {
		It("is date-based and week-mapped off the period start", func() {
			sp.Calendars = []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(2), To: weekDate(5), Effect: EffectBlockStart},
			}
			ws := parseCalendars(sp, 26)
			Expect(ws).To(HaveLen(1))
			Expect(ws[0].fromWeek).To(Equal(2))
			// To is inclusive per §7's fromDate/toDate pair: a freeze "to" w5
			// covers week 5, and work resumes at w6.
			Expect(ws[0].toWeek).To(Equal(5))
		})

		It("ignores windows outside the period rather than inventing weeks", func() {
			sp.Calendars = []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: "2020-01-06", To: "2020-01-13", Effect: EffectBlockStart},
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: "2030-01-06", To: "2030-01-13", Effect: EffectBlockStart},
			}
			ws := parseCalendars(sp, 26)
			Expect(ws).To(BeEmpty())
		})

		It("keeps windows without a period start inert (no week mapping exists)", func() {
			sp.Calendars = []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: "2026-01-05", To: "2026-01-12", Effect: EffectBlockStart},
			}
			sp.PeriodStart = ""
			ws := parseCalendars(sp, 26)
			Expect(ws).To(BeEmpty(), "a date cannot become a week without a period start")
		})
	})

	Describe("NFR-006: no produced schedule violates the windows", func() {
		It("holds across the demo plan with a freeze over its middle", func() {
			dTeams, dInits := Demo()
			dsp := DemoScheduling()
			dsp.Calendars = []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(8), To: weekDate(12), Effect: EffectBlockStart},
			}
			s := ComputeSchedule(dTeams, dInits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, dsp)
			frozen := frozenWeeks(parseCalendars(dsp, 26), EffectBlockStart)
			for _, si := range s.Initiatives {
				for _, sl := range si.Slices {
					Expect(frozen[sl.StartWeek]).To(BeFalse(),
						"%s/%s starts in a frozen week", si.Name, sl.Pod)
				}
			}
		})
	})
})

// frozenWeeks is the test-side set of weeks a given effect rules out.
func frozenWeeks(ws []calendarWindow, effect string) map[int]bool {
	out := map[int]bool{}
	for _, w := range ws {
		if w.effect != effect {
			continue
		}
		for k := w.fromWeek; k <= w.toWeek; k++ {
			out[k] = true
		}
	}
	return out
}

// The P1 regressions of the first review round, each pinned to the shape that
// was broken.
var _ = Describe("calendar window edge cases", func() {
	weekDate := func(w int) string {
		t0, _ := time.Parse(isoDate, specPeriodStart)
		return t0.AddDate(0, 0, w*7).Format(isoDate)
	}
	dayDate := func(days int) string {
		t0, _ := time.Parse(isoDate, specPeriodStart)
		return t0.AddDate(0, 0, days).Format(isoDate)
	}

	It("maps a window ending days before the period start to a negative week, not week 0", func() {
		sp := SchedulingParams{PeriodStart: specPeriodStart, Calendars: []CalendarWindow{
			// Entirely before the period, ending 3 days before week 0.
			{Kind: CalChangeFreeze, Scope: ScopeOrg, From: dayDate(-17), To: dayDate(-3), Effect: EffectBlockStart},
		}}
		Expect(parseCalendars(sp, 26)).To(BeEmpty(),
			"a pre-period window must be inert, not a freeze on week 0")
	})

	It("ignores a window starting at the horizon: the period ends before it", func() {
		sp := SchedulingParams{PeriodStart: specPeriodStart, Calendars: []CalendarWindow{
			{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(26), To: weekDate(30), Effect: EffectBlockStart},
		}}
		Expect(parseCalendars(sp, 26)).To(BeEmpty())
	})

	It("applies a site-scoped freeze only to that site's pods", func() {
		teams := []Team{
			{Name: "Near", Devs: 4, Tracks: 2, Site: "Kraków"},
			{Name: "Far", Devs: 4, Tracks: 2, Site: "Toronto"},
		}
		inits := []Initiative{
			{Name: "Home", Work: map[string]TeamWork{"Near": podWork(4)}, StatedPriority: 1},
			{Name: "Away", Work: map[string]TeamWork{"Far": podWork(4)}, StatedPriority: 2},
		}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25),
			Calendars: []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: "Kraków", From: weekDate(0), To: weekDate(3), Effect: EffectBlockStart},
			}}
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		byName := map[string]ScheduledInitiative{}
		for _, si := range s.Initiatives {
			byName[si.Name] = si
		}
		Expect(byName["Home"].StartWeek).To(Equal(4), "Kraków is frozen for weeks 0-3")
		Expect(byName["Away"].StartWeek).To(Equal(0), "Toronto starts regardless")
	})

	It("reduces capacity org-wide when the scope is org", func() {
		teams := []Team{{Name: "Delta", Devs: 4, Tracks: 2, Site: "Kraków"}}
		inits := []Initiative{{Name: "Dated", Work: map[string]TeamWork{"Delta": podWork(4)}, StatedPriority: 1}}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25),
			Calendars: []CalendarWindow{
				{Kind: CalEvent, Scope: ScopeOrg, From: weekDate(0), To: weekDate(3), Effect: EffectReduceCapacity},
			}}
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		Expect(s.Initiatives[0].StartWeek).To(Equal(4), "an org-wide shutdown is a shutdown for every pod")
	})

	It("does not move carryover work past a freeze covering week 0 (AC X.4)", func() {
		teams := []Team{{Name: "Delta", Devs: 4, Tracks: 2}}
		inits := []Initiative{{Name: "Carry", Work: map[string]TeamWork{"Delta": podWork(4)},
			StatedPriority: 1, InFlight: true, ProgressPct: 0.5}}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25),
			Calendars: []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(0), To: weekDate(3), Effect: EffectBlockStart},
			}}
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		Expect(s.Initiatives[0].StartWeek).To(Equal(0),
			"carryover began before the period; a freeze cannot un-start it")
	})

	It("keeps the estimated weeks when a block-finish moves the completion", func() {
		teams := []Team{{Name: "Delta", Devs: 4, Tracks: 2}}
		inits := []Initiative{{Name: "Dated", Work: map[string]TeamWork{"Delta": podWork(4)}, StatedPriority: 1}}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25),
			Calendars: []CalendarWindow{
				// The finish would land at w4; the freeze holds w3-5, so the
				// completion moves to w6 — with the same 4 estimated weeks.
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(3), To: weekDate(5), Effect: EffectBlockFinish},
			}}
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		sl := s.Initiatives[0].Slices[0]
		Expect(sl.FinishWeek).To(Equal(6), "the first legal completion week")
		Expect(sl.RemainingWeeks).To(Equal(float64(4)), "the freeze adds waiting, not scope")
	})

	It("does not report holiday weeks as busy occupancy", func() {
		teams := []Team{{Name: "Delta", Devs: 4, Tracks: 2, Site: "Kraków"}}
		inits := []Initiative{{Name: "Dated", Work: map[string]TeamWork{"Delta": podWork(4)}, StatedPriority: 1}}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25),
			Calendars: []CalendarWindow{
				{Kind: CalSiteNonWorking, Scope: "Kraków", From: weekDate(1), To: weekDate(2), Effect: EffectReduceCapacity},
			}}
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		var delta *PodSchedule
		for i := range s.PodWeeks {
			if s.PodWeeks[i].Pod == "Delta" {
				delta = &s.PodWeeks[i]
			}
		}
		Expect(delta).NotTo(BeNil())
		Expect(delta.Weeks[0].Busy).To(Equal(1), "week 0 works")
		Expect(delta.Weeks[1].Busy).To(Equal(0), "week 1 is a holiday, not work")
		Expect(delta.Weeks[2].Busy).To(Equal(0), "week 2 is a holiday, not work")
		Expect(delta.Weeks[3].Busy).To(Equal(1), "work resumes after the holiday")
	})
})


var _ = Describe("calendar window edge cases, round two", func() {
	weekDate := func(w int) string {
		t0, _ := time.Parse(isoDate, specPeriodStart)
		return t0.AddDate(0, 0, w*7).Format(isoDate)
	}

	It("never starts a slice in a block-start week reached after reduced-capacity weeks", func() {
		// A holiday w0-w2 walks the span forward; w3 is frozen for starts.
		// The start must be w4, not the frozen w3 the walk reaches first.
		teams := []Team{{Name: "Delta", Devs: 4, Tracks: 2, Site: "Kraków"}}
		inits := []Initiative{{Name: "Dated", Work: map[string]TeamWork{"Delta": podWork(2)}, StatedPriority: 1}}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25),
			Calendars: []CalendarWindow{
				{Kind: CalSiteNonWorking, Scope: "Kraków", From: weekDate(0), To: weekDate(2), Effect: EffectReduceCapacity},
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(3), To: weekDate(3), Effect: EffectBlockStart},
			}}
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		sl := s.Initiatives[0].Slices[0]
		Expect(sl.StartWeek).To(Equal(4), "the frozen week after the holiday is not a start")
	})

	It("does not book block-finish waiting weeks as occupancy", func() {
		// The finish w4 lands in a freeze w3-w5, so the completion moves to
		// w6 — but weeks 4-5 are waiting, not work: the pod must not read
		// busy through them.
		teams := []Team{{Name: "Delta", Devs: 4, Tracks: 2}}
		inits := []Initiative{{Name: "Dated", Work: map[string]TeamWork{"Delta": podWork(4)}, StatedPriority: 1}}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25),
			Calendars: []CalendarWindow{
				{Kind: CalChangeFreeze, Scope: ScopeOrg, From: weekDate(3), To: weekDate(5), Effect: EffectBlockFinish},
			}}
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		var delta *PodSchedule
		for i := range s.PodWeeks {
			if s.PodWeeks[i].Pod == "Delta" {
				delta = &s.PodWeeks[i]
			}
		}
		Expect(delta).NotTo(BeNil())
		// Guard the premise: without the freeze moving the finish, weeks 4-5
		// would read empty for a plain 4-week slice and the assertions below
		// would pass vacuously.
		Expect(delta.Slices[0].FinishWeek).To(Equal(6), "the freeze moved the completion to w6")
		// The 4 estimated weeks are w0-w3; the finish moved from w4 to w6, so
		// weeks 4-5 are freeze waiting and must not book. Week 3 is work.
		Expect(delta.Weeks[3].Busy).To(Equal(1), "the last estimated week is work")
		for w := 4; w <= 5; w++ {
			Expect(delta.Weeks[w].Busy).To(Equal(0),
				"week %d is freeze waiting, not work", w)
		}
		Expect(delta.Weeks[0].Busy).To(Equal(1), "the work before the freeze is booked")
	})
})
