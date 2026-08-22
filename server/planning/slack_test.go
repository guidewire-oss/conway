package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Stories 8-9's server-side additions: WorkSlice carries the in-plan dependency
// edges (FR-037's arrows, FR-042's upstream/downstream naming) and the
// latest-start/slack pair (FR-041, AC 9.2-9.3).
//
// The fixtures are small enough to check by hand: a diamond chain on three
// pods where the middle pod's slice is the only one that can wait.
var _ = Describe("slice slack and dependencies", func() {
	var (
		teams []Team
		inits []Initiative
		sp    SchedulingParams
	)

	BeforeEach(func() {
		teams = []Team{
			{Name: "Alpha", Devs: 4, Tracks: 2},
			{Name: "Mid", Devs: 4, Tracks: 2},
			{Name: "Omega", Devs: 4, Tracks: 2},
		}
		inits = []Initiative{
			// A 4-week chain with one parallel branch: Alpha -> Mid -> Omega is
			// the critical path; Alpha -> Omega directly is the short branch.
			{Name: "Chain", Work: map[string]TeamWork{
				"Alpha": podWork(4),
				"Mid":   podWork(4, "Alpha"),
				"Omega": podWork(2, "Alpha"),
			}, StatedPriority: 1},
		}
		sp = SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff, BufferPct: pctOf(0.25)}
	})

	schedule := func() *Schedule {
		return ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
	}

	sliceOf := func(s *Schedule, pod string) WorkSlice {
		for _, si := range s.Initiatives {
			for _, sl := range si.Slices {
				if sl.Pod == pod {
					return sl
				}
			}
		}
		Fail("no slice at pod " + pod)
		return WorkSlice{}
	}

	It("carries the in-plan dependency edges on each slice", func() {
		s := schedule()
		Expect(sliceOf(s, "Mid").DependsOn).To(ConsistOf("Alpha"))
		Expect(sliceOf(s, "Omega").DependsOn).To(ConsistOf("Alpha"))
		Expect(sliceOf(s, "Alpha").DependsOn).To(BeEmpty())
	})

	It("gives the critical chain zero slack (AC 9.3)", func() {
		s := schedule()
		// Alpha runs w0-w4, Mid w4-w8: both are on the critical chain, so
		// delaying either moves the finish.
		Expect(sliceOf(s, "Alpha").SlackWeeks).To(Equal(0))
		Expect(sliceOf(s, "Mid").SlackWeeks).To(Equal(0))
	})

	It("gives the parallel short branch the weeks it can wait (AC 9.2)", func() {
		s := schedule()
		// Omega (2w, Alpha->Omega) could start at w4 and finish at w6, while
		// the chain finishes at w8: its latest start is w6, slack 2.
		omega := sliceOf(s, "Omega")
		Expect(omega.StartWeek).To(Equal(4))
		Expect(omega.LatestStartWeek).To(Equal(6))
		Expect(omega.SlackWeeks).To(Equal(2))
	})

	It("never lets slack eat the buffer's weeks", func() {
		// Decision 14: the buffer protects the commit date, so the backward
		// pass anchors on the raw finish. Latest start may not pass it.
		s := schedule()
		chain := s.Initiatives[0]
		for _, sl := range chain.Slices {
			Expect(sl.StartWeek + sl.SlackWeeks).To(Equal(sl.LatestStartWeek))
			Expect(sl.LatestStartWeek).To(BeNumerically("<=", chain.RawFinishWeek),
				"latest start may never eat the buffer")
		}
	})

	It("carries the same slack on the per-pod schedule's copies", func() {
		// The pod view (FR-040) reads PodWeeks' slice copies, built during the
		// run — before the annotation pass. The two must never disagree.
		dTeams, dInits := Demo()
		s := ComputeSchedule(dTeams, dInits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, DemoScheduling())
		byPod := map[string]map[string]WorkSlice{} // pod -> initiative -> copy
		for _, ps := range s.PodWeeks {
			byPod[ps.Pod] = map[string]WorkSlice{}
			for _, sl := range ps.Slices {
				byPod[ps.Pod][sl.Initiative] = sl
			}
		}
		checked := 0
		for _, si := range s.Initiatives {
			for _, sl := range si.Slices {
				cp, ok := byPod[sl.Pod][si.Name]
				Expect(ok).To(BeTrue(), "%s/%s missing from PodWeeks", si.Name, sl.Pod)
				Expect(cp.LatestStartWeek).To(Equal(sl.LatestStartWeek),
					"%s/%s: pod copy disagrees with the initiative slice", si.Name, sl.Pod)
				Expect(cp.SlackWeeks).To(Equal(sl.SlackWeeks))
				Expect(cp.DependsOn).To(Equal(sl.DependsOn))
				checked++
			}
		}
		Expect(checked).To(BeNumerically(">", 0))
	})

	It("marks zero slack distinctly from movable work (AC 9.3)", func() {
		s := schedule()
		var zero, movable int
		for _, si := range s.Initiatives {
			for _, sl := range si.Slices {
				switch sl.SlackWeeks {
				case 0:
					zero++
				default:
					movable++
				}
			}
		}
		Expect(zero).To(Equal(2), "Alpha and Mid are the critical chain")
		Expect(movable).To(Equal(1), "Omega has room to wait")
	})

	It("holds for the demo plan: every slice has consistent slack arithmetic", func() {
		dTeams, dInits := Demo()
		s := ComputeSchedule(dTeams, dInits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, DemoScheduling())
		Expect(s.Initiatives).NotTo(BeEmpty())
		for _, si := range s.Initiatives {
			for _, sl := range si.Slices {
				Expect(sl.LatestStartWeek).To(BeNumerically(">=", sl.StartWeek),
					"%s/%s: latest start is never before the actual start", si.Name, sl.Pod)
				Expect(sl.SlackWeeks).To(Equal(sl.LatestStartWeek - sl.StartWeek))
				Expect(sl.LatestStartWeek).To(BeNumerically("<=", si.RawFinishWeek),
					"%s/%s: slack may not eat the buffer", si.Name, sl.Pod)
			}
		}
	})

	It("survives an initiative with no dependencies at all", func() {
		inits = []Initiative{{Name: "Solo", Work: map[string]TeamWork{"Alpha": podWork(3)}, StatedPriority: 1}}
		s := schedule()
		sl := sliceOf(s, "Alpha")
		Expect(sl.DependsOn).To(BeEmpty())
		Expect(sl.SlackWeeks).To(Equal(0), "a lone slice is its own critical chain")
	})
})
