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
