package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Decision 28: the schedule stops at the chosen horizon. Before this, a plan
// loaded to its capacity kept pushing work outward until it fitted -- week 606 on
// a 26-week horizon, 1.03MB of mostly-idle pod calendar, and every verdict reading
// "no-date" because no verdict was relative to the period.
var _ = Describe("the horizon bound", func() {
	// Two pods, one track each, and far more work than eight weeks can hold.
	oversubscribed := func(horizon float64) (*Schedule, []Initiative) {
		teams := []Team{{Name: "Alpha", Tracks: 1}, {Name: "Beta", Tracks: 1}}
		inits := []Initiative{}
		for i, name := range []string{"one", "two", "three", "four", "five", "six"} {
			pod := "Alpha"
			if i%2 == 1 {
				pod = "Beta"
			}
			inits = append(inits, Initiative{
				Name: name, Work: map[string]TeamWork{pod: podWork(4)},
				StatedPriority: i + 1, PriorityLocked: true,
			})
		}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipStrict, BufferPct: pctOf(0)}
		return ComputeScheduleWith(teams, inits, Params{HorizonWeeks: horizon}, sp,
			ScheduleOptions{}), inits
	}

	// The invariant is about starts, not finishes: work already running when the
	// period ends is real and occupies pods, so hiding it would be the same dishonesty
	// in the other direction. Nothing may *begin* outside the period.
	It("starts nothing outside the period", func() {
		sched, _ := oversubscribed(8)
		for _, si := range sched.Initiatives {
			if si.Verdict == verdictBeyondHorizon {
				continue
			}
			Expect(si.StartWeek).To(BeNumerically("<", 8),
				si.Name+" began outside the period")
		}
		for _, ps := range sched.PodWeeks {
			for _, sl := range ps.Slices {
				Expect(sl.StartWeek).To(BeNumerically("<", 8),
					sl.Initiative+" slice on "+ps.Pod+" began outside the period")
			}
		}
	})

	It("reports what did not fit as beyond-horizon, with no invented weeks", func() {
		sched, inits := oversubscribed(8)
		var beyond []ScheduledInitiative
		for _, si := range sched.Initiatives {
			if si.Verdict == verdictBeyondHorizon {
				beyond = append(beyond, si)
			}
		}
		Expect(beyond).NotTo(BeEmpty(), "six 4-week slices cannot fit two tracks in 8 weeks")
		Expect(len(beyond)).To(BeNumerically("<", len(inits)), "and some must still fit")
		for _, si := range beyond {
			Expect(si.StartWeek).To(BeZero(), si.Name+" must carry no start week")
			Expect(si.CommitWeek).To(BeZero(), si.Name+" must carry no commit week")
		}
	})

	// The point of the decision: a planner is told the shortfall in terms they can
	// act on, rather than being shown a week number a decade out.
	It("reports the demand against the capacity that explains the shortfall", func() {
		sched, _ := oversubscribed(8)
		Expect(sched.Fit).NotTo(BeNil())
		Expect(sched.Fit.BeyondHorizon).To(BeNumerically(">", 0))
		// Two tracks over eight weeks, no capacity loss.
		Expect(sched.Fit.TrackWeeksAvailable).To(BeNumerically("~", 16, 0.001))
		// Six initiatives of four weeks each.
		Expect(sched.Fit.PodWeeksDemanded).To(BeNumerically("~", 24, 0.001))
	})

	// Bounded, not truncated: the calendar may run past the period only as far as
	// work that started inside it needs. Here the longest slice is four weeks, so
	// nothing can reach beyond week 11 from a start at week 7. Before this decision
	// the same shape produced calendars hundreds of weeks long.
	It("bounds the pod calendar to the period plus the work that started in it", func() {
		sched, _ := oversubscribed(8)
		for _, ps := range sched.PodWeeks {
			Expect(len(ps.Weeks)).To(BeNumerically("<=", 8+4),
				ps.Pod+" carries calendar further past the period than any in-period slice needs")
		}
	})

	// A plan that fits must be untouched: this decision is about the overflow, not
	// about changing any schedule that was already inside its period.
	It("leaves a plan that fits exactly as it was", func() {
		teams := []Team{{Name: "Alpha", Tracks: 2}}
		inits := []Initiative{
			{Name: "first", Work: map[string]TeamWork{"Alpha": podWork(4)}, StatedPriority: 1, PriorityLocked: true},
			{Name: "second", Work: map[string]TeamWork{"Alpha": podWork(4)}, StatedPriority: 2, PriorityLocked: true},
		}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipStrict, BufferPct: pctOf(0.25)}
		sched := ComputeScheduleWith(teams, inits, Params{HorizonWeeks: 26}, sp, ScheduleOptions{})

		for _, si := range sched.Initiatives {
			Expect(si.Verdict).NotTo(Equal(verdictBeyondHorizon), si.Name)
			Expect(si.CommitWeek).To(BeNumerically(">", 0))
		}
		Expect(sched.Fit.BeyondHorizon).To(BeZero())
	})
})
