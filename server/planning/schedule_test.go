package planning

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// specPeriodStart is a Monday, so week 0 begins on a working day.
const specPeriodStart = "2026-01-05"

// weekDate is the ISO date exactly n weeks after the period start, so a spec can
// express "target week 16" the way a planner enters it — as a date on the sheet.
func weekDate(n int) string {
	t0, err := time.Parse("2006-01-02", specPeriodStart)
	Expect(err).NotTo(HaveOccurred())
	return t0.AddDate(0, 0, n*7).Format("2006-01-02")
}

// pctOf exists because bufferPct is a pointer: an explicit 0 means "commit on
// the raw finish" and must be distinguishable from an absent value, which
// defaults to 0.25 (spec 001 §11 D20, amended 2026-08-19).
func pctOf(f float64) *float64 { return &f }

func podWork(weeks float64, deps ...string) TeamWork {
	return TeamWork{Weeks: weeks, Estimated: true, InPath: true, DependsOn: deps}
}

func scheduledFor(s *Schedule, name string) *ScheduledInitiative {
	for i := range s.Initiatives {
		if s.Initiatives[i].Name == name {
			return &s.Initiatives[i]
		}
	}
	Fail("no scheduled initiative named " + name)
	return nil
}

func podScheduleFor(s *Schedule, pod string) *PodSchedule {
	for i := range s.PodWeeks {
		if s.PodWeeks[i].Pod == pod {
			return &s.PodWeeks[i]
		}
	}
	Fail("no pod schedule for " + pod)
	return nil
}

func sliceAt(si *ScheduledInitiative, pod string) *WorkSlice {
	for i := range si.Slices {
		if si.Slices[i].Pod == pod {
			return &si.Slices[i]
		}
	}
	Fail("initiative " + si.Name + " has no slice at " + pod)
	return nil
}

var _ = Describe("ComputeSchedule", func() {
	// Weeks are integers from the period start; a finish week is exclusive, so a
	// 6-week slice starting in week 5 occupies weeks 5-10 and finishes in week 11.

	Describe("Story 1 — proposing an execution order", func() {
		// AC 1.2 and Decision 1: the whole reason for finite-capacity scheduling is
		// that the order work can actually run in is not the order ranking prefers.
		Context("when the ranking order and the capacity-feasible order differ", func() {
			var sched *Schedule

			BeforeEach(func() {
				teams := []Team{
					{Name: "Delta", Tracks: 1, Site: "Remote"}, // the drum: one track
					{Name: "Atlas", Tracks: 2, Site: "Austin"},
				}
				inits := []Initiative{
					{
						Name: "Payments GA",
						Work: map[string]TeamWork{
							"Atlas": podWork(5),
							"Delta": podWork(6, "Atlas"),
						},
						Tier: 1, CostOfDelayPerWeek: 9, TargetDate: weekDate(16),
					},
					{
						Name: "Search revamp",
						Work: map[string]TeamWork{"Delta": podWork(2)},
						Tier: 3, CostOfDelayPerWeek: 2,
					},
				}
				sched = ComputeSchedule(teams, inits,
					Params{HorizonWeeks: 26, CapacityLoss: 0},
					SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 2, BufferPct: pctOf(0.25)})
			})

			It("ranks the valuable dated initiative first", func() {
				Expect(scheduledFor(sched, "Payments GA").ProposedRank).To(Equal(1))
				Expect(scheduledFor(sched, "Search revamp").ProposedRank).To(Equal(2))
			})

			It("still runs the second-ranked work first at the drum, because the first cannot start there yet", func() {
				payments := sliceAt(scheduledFor(sched, "Payments GA"), "Delta")
				search := sliceAt(scheduledFor(sched, "Search revamp"), "Delta")
				Expect(payments.StartWeek).To(Equal(5)) // waits on its own Atlas slice
				Expect(search.StartWeek).To(Equal(0))

				delta := podScheduleFor(sched, "Delta")
				Expect(delta.Slices).To(HaveLen(2))
				Expect(delta.Slices[0].Initiative).To(Equal("Search revamp"))
				Expect(delta.Slices[1].Initiative).To(Equal("Payments GA"))
			})

			It("does not idle the drum waiting for the top-ranked initiative", func() {
				Expect(podScheduleFor(sched, "Delta").Weeks[0].Busy).To(Equal(1))
			})

			It("names the constraint pod as the drum", func() {
				Expect(sched.DrumPods).To(ContainElement("Delta"))
			})

			It("commits on the buffered finish, never the raw one (Decision 9)", func() {
				p := scheduledFor(sched, "Payments GA")
				Expect(p.StartWeek).To(Equal(0))
				Expect(p.RawFinishWeek).To(Equal(11))
				Expect(p.BufferWeeks).To(Equal(3)) // 25% of an 11-week chain, rounded up
				Expect(p.CommitWeek).To(Equal(14))
				Expect(p.Verdict).To(Equal("on-time")) // commit 14 is inside target 16
			})

			It("reports no date rather than inventing a verdict", func() {
				Expect(scheduledFor(sched, "Search revamp").Verdict).To(Equal("no-date"))
			})
		})

		// AC 1.4
		It("produces an identical result on a second run", func() {
			teams, inits := Demo()
			params := Params{HorizonWeeks: 26, CapacityLoss: 0.10}
			sp := SchedulingParams{PeriodStart: specPeriodStart, BufferPct: pctOf(0.25)}
			Expect(ComputeSchedule(teams, inits, params, sp)).To(Equal(ComputeSchedule(teams, inits, params, sp)))
		})

		// AC 1.5
		It("caps work in progress at release and names the limit as the reason", func() {
			teams := []Team{{Name: "Atlas", Tracks: 6}}
			var inits []Initiative
			for _, n := range []string{"One", "Two", "Three", "Four", "Five", "Six"} {
				inits = append(inits, Initiative{Name: n, Work: map[string]TeamWork{"Atlas": podWork(4)}})
			}
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 4, BufferPct: pctOf(0.25)})

			started := 0
			for _, si := range sched.Initiatives {
				if si.StartWeek == 0 {
					started++
				}
			}
			Expect(started).To(Equal(4), "Atlas has 6 free tracks, so only the WIP limit can hold work back")

			held := scheduledFor(sched, "Five")
			Expect(held.StartWeek).To(Equal(4))
			Expect(held.BindingConstraint).To(Equal("wip-limit"))
		})
	})

	Describe("Story 2 — testing a committed date", func() {
		// AC 2.3 and Decision 12: "no ordering could meet this" is a different
		// conversation from "we are fighting over a pod", so they cannot share a verdict.
		Context("when a target date cannot be met", func() {
			var sched *Schedule

			BeforeEach(func() {
				teams := []Team{
					{Name: "PodA", Tracks: 3}, {Name: "PodB", Tracks: 3}, {Name: "PodC", Tracks: 3},
					{Name: "Delta", Tracks: 1},
				}
				inits := []Initiative{
					{Name: "DR for event streaming", Work: map[string]TeamWork{
						"PodA": podWork(10), "PodB": podWork(10, "PodA"), "PodC": podWork(10, "PodB"),
					}, TargetDate: weekDate(12), Tier: 2, CostOfDelayPerWeek: 5},
					{Name: "Drum hog", Work: map[string]TeamWork{"Delta": podWork(6)},
						StatedPriority: 1, PriorityLocked: true, Tier: 1, CostOfDelayPerWeek: 8},
					{Name: "Squeezed out", Work: map[string]TeamWork{"Delta": podWork(4)},
						StatedPriority: 2, PriorityLocked: true, Tier: 3, CostOfDelayPerWeek: 2,
						TargetDate: weekDate(6)},
				}
				sched = ComputeSchedule(teams, inits,
					Params{HorizonWeeks: 40, CapacityLoss: 0},
					SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 3, BufferPct: pctOf(0.25)})
			})

			It("calls a 30-week chain against a week-12 date structurally infeasible", func() {
				dr := scheduledFor(sched, "DR for event streaming")
				Expect(dr.RawFinishWeek).To(Equal(30), "capacity is ample; only its own chain sets this")
				Expect(dr.Verdict).To(Equal("structurally-infeasible"))
			})

			It("calls a date lost to contention merely late", func() {
				sq := scheduledFor(sched, "Squeezed out")
				Expect(sq.StartWeek).To(Equal(6)) // the drum was busy until then
				Expect(sq.Verdict).To(Equal("late"))
				Expect(sq.WeeksLate).To(Equal(5)) // commit 11 against target 6
			})
		})

		// AC 2.5
		Context("when an in-path pod carries no estimate", func() {
			It("schedules the estimated work, names the gap and marks the verdict provisional", func() {
				teams := []Team{{Name: "Atlas", Tracks: 2}, {Name: "Delta", Tracks: 1}}
				inits := []Initiative{{
					Name: "Half-estimated", Work: map[string]TeamWork{
						"Atlas": podWork(4),
						"Delta": {InPath: true, DependsOn: []string{"Atlas"}}, // TBD on the sheet
					},
					TargetDate: weekDate(10), Tier: 2, CostOfDelayPerWeek: 4,
				}}
				sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
					SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 2, BufferPct: pctOf(0.25)})

				si := scheduledFor(sched, "Half-estimated")
				Expect(si.RawFinishWeek).To(Equal(4))
				Expect(si.Provisional).To(BeTrue())
				Expect(si.UnestimatedPods).To(ConsistOf("Delta"))
				Expect(si.Verdict).To(Equal("on-time"))
			})
		})
	})

	Describe("Story 3 — protecting a non-negotiable priority", func() {
		// AC 3.1 with FR-013: a lock is a constraint, and the report has to be able
		// to say what honouring it costs.
		Context("when a locked priority forces a worse order", func() {
			var free, locked *Schedule

			BeforeEach(func() {
				teams := []Team{{Name: "Delta", Tracks: 1}}
				build := func(lock bool) []Initiative {
					return []Initiative{
						{Name: "Regulatory reporting", Work: map[string]TeamWork{"Delta": podWork(6)},
							Tier: 1, CostOfDelayPerWeek: 10, StatedPriority: 2, TargetDate: weekDate(8)},
						{Name: "Internal tooling", Work: map[string]TeamWork{"Delta": podWork(6)},
							Tier: 4, CostOfDelayPerWeek: 1, StatedPriority: 1, PriorityLocked: lock},
					}
				}
				params := Params{HorizonWeeks: 26, CapacityLoss: 0}
				sp := SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 2, BufferPct: pctOf(0.25)}
				free = ComputeSchedule(teams, build(false), params, sp)
				locked = ComputeSchedule(teams, build(true), params, sp)
			})

			It("overrules the stated order when nothing is locked", func() {
				r := scheduledFor(free, "Regulatory reporting")
				Expect(r.ProposedRank).To(Equal(1))
				Expect(r.StatedRank).To(Equal(2))
				Expect(r.Verdict).To(Equal("on-time"))
			})

			It("pins a locked initiative to its stated rank", func() {
				Expect(scheduledFor(locked, "Internal tooling").ProposedRank).To(Equal(1))
			})

			It("makes the objective measurably worse, and says which date breaks", func() {
				Expect(locked.ObjectiveScore).To(BeNumerically(">", free.ObjectiveScore))
				r := scheduledFor(locked, "Regulatory reporting")
				Expect(r.Verdict).To(Equal("late"))
				Expect(r.WeeksLate).To(Equal(6))
			})

			// FR-013: both scores, so the planner can see the price of their own order.
			It("prices the planner's stated order against the proposal", func() {
				Expect(free.StatedOrderObjectiveScore).To(BeNumerically(">", free.ObjectiveScore))
				Expect(free.StatedOrderObjectiveScore).To(Equal(locked.ObjectiveScore))
			})
		})
	})

	Describe("Story 4 — seeing what set a start week", func() {
		// AC 2.2 with FR-003: the binding constraint is the entire explanation, so
		// "your track was busy" and "your own upstream was not done" must not blur.
		Context("when work is held back", func() {
			var sched *Schedule

			BeforeEach(func() {
				teams := []Team{{Name: "Delta", Tracks: 1}, {Name: "Atlas", Tracks: 2}, {Name: "Granite", Tracks: 2}}
				inits := []Initiative{
					{Name: "First at the drum", Work: map[string]TeamWork{"Delta": podWork(4)},
						StatedPriority: 1, PriorityLocked: true},
					{Name: "Second at the drum", Work: map[string]TeamWork{"Delta": podWork(4)},
						StatedPriority: 2, PriorityLocked: true},
					{Name: "Waits on itself", Work: map[string]TeamWork{
						"Atlas": podWork(3), "Granite": podWork(2, "Atlas")},
						StatedPriority: 3, PriorityLocked: true},
				}
				sched = ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
					SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 3, BufferPct: pctOf(0.25)})
			})

			It("blames pod capacity when a free track was the only thing missing", func() {
				second := scheduledFor(sched, "Second at the drum")
				Expect(second.StartWeek).To(Equal(4))
				Expect(second.BindingConstraint).To(Equal("pod-capacity"))
				Expect(sliceAt(second, "Delta").WaitWeeks).To(Equal(4.0))
			})

			It("blames the dependency when the pod was free and the upstream was not", func() {
				granite := sliceAt(scheduledFor(sched, "Waits on itself"), "Granite")
				Expect(granite.StartWeek).To(Equal(3))
				Expect(granite.BindingConstraint).To(Equal("dependency"))
			})

			It("blames nothing for work that starts immediately", func() {
				first := scheduledFor(sched, "First at the drum")
				Expect(first.StartWeek).To(Equal(0))
				Expect(first.BindingConstraint).To(BeEmpty())
				Expect(sliceAt(first, "Delta").WaitWeeks).To(Equal(0.0))
			})

			// FR-004: per-pod weekly utilization, which the heatmap in §13 reads.
			It("reports each pod's week-by-week load and the initiatives in it", func() {
				delta := podScheduleFor(sched, "Delta")
				Expect(delta.Tracks).To(Equal(1))
				Expect(delta.Weeks[0].Utilization).To(Equal(1.0))
				Expect(delta.Weeks[0].Initiatives).To(ConsistOf("First at the drum"))
				Expect(delta.Weeks[4].Initiatives).To(ConsistOf("Second at the drum"))
				Expect(delta.Weeks[8].Busy).To(Equal(0))
			})
		})
	})

	// FR-012: the reason has to describe the rule that actually produced the order.
	// Citing the tardiness index on a plan where constraint-first won is an
	// explanation that is confidently wrong, which is worse than none.
	It("explains a rank deviation in terms of the rule that won", func() {
		teams, inits := Demo()
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1},
			DemoScheduling())

		Expect(sched.Reconciliation).NotTo(BeEmpty())
		basis := ruleBasis(sched.Rule)
		Expect(basis).NotTo(Equal("the winning dispatch rule"), "the winning rule should be a named one")
		for _, r := range sched.Reconciliation {
			if strings.Contains(r.Reason, "locked") {
				continue
			}
			Expect(r.Reason).To(ContainSubstring(basis),
				"%s cites a rule other than the winning %s", r.Initiative, sched.Rule)
		}
	})

	// The JS view is tested against a committed dump of this type
	// (tests/fixtures/schedule-demo.json). JS cannot notice a Go field rename, so
	// this spec is the tripwire: it fails when the fixture no longer describes this
	// package, which is the signal to regenerate it.
	It("still matches the committed fixture the JS view is tested against", func() {
		blob, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "schedule-demo.json"))
		Expect(err).NotTo(HaveOccurred())

		var fixture Schedule
		dec := json.NewDecoder(bytes.NewReader(blob))
		dec.DisallowUnknownFields() // a removed or renamed field shows up here
		Expect(dec.Decode(&fixture)).To(Succeed())

		// Fields the view actually reads. A rename that json tags away silently would
		// leave these zero rather than failing to decode.
		Expect(fixture.Initiatives).NotTo(BeEmpty())
		Expect(fixture.PodWeeks).NotTo(BeEmpty())
		Expect(fixture.Rule).NotTo(BeEmpty())
		Expect(fixture.DrumPods).NotTo(BeEmpty())
		Expect(fixture.WipLimit.Value).To(BeNumerically(">", 0))
		Expect(fixture.Reconciliation).NotTo(BeEmpty())
		Expect(fixture.Initiatives[0].Slices).NotTo(BeEmpty())
		Expect(fixture.PodWeeks[0].Weeks).NotTo(BeEmpty())
		// The timeline's slack pair (FR-041): the fixture must carry them, or
		// the JS view is being tested against a schedule that predates them.
		depSeen, slackSeen := false, false
		for _, si := range fixture.Initiatives {
			for _, sl := range si.Slices {
				if len(sl.DependsOn) > 0 {
					depSeen = true
				}
				// SlackWeeks itself, not LatestStartWeek: any slice starting
				// after week 0 has a positive latest start even at zero slack,
				// which would let an all-zero-slack fixture pass.
				if sl.SlackWeeks > 0 {
					slackSeen = true
				}
				Expect(sl.SlackWeeks).To(Equal(sl.LatestStartWeek - sl.StartWeek))
			}
		}
		Expect(depSeen).To(BeTrue(), "the demo plan has cross-pod dependencies; arrows need the edges")
		Expect(slackSeen).To(BeTrue(), "the demo plan has parallel branches; the pod view needs real slack")
	})

	// D22 as amended 2026-08-20: the planner picks which rule the org limit follows,
	// because the limit encodes a belief about multitasking that the schedule cannot
	// confirm — Decision 4 removed the very term that would price it.
	Describe("the WIP model", func() {
		// One drum-consuming initiative and two that never touch the drum, so the models
		// have something to disagree about.
		build := func() ([]Team, []Initiative) {
			teams := []Team{{Name: "Delta", Tracks: 1}, {Name: "Far", Tracks: 4}}
			inits := []Initiative{
				{Name: "Needs the drum", Work: map[string]TeamWork{"Delta": podWork(6)},
					StatedPriority: 1, PriorityLocked: true},
				{Name: "Away from the drum A", Work: map[string]TeamWork{"Far": podWork(4)},
					StatedPriority: 2, PriorityLocked: true},
				{Name: "Away from the drum B", Work: map[string]TeamWork{"Far": podWork(4)},
					StatedPriority: 3, PriorityLocked: true},
			}
			return teams, inits
		}
		run := func(model string) *Schedule {
			teams, inits := build()
			return ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, WipModel: model, BufferPct: pctOf(0.25)})
		}

		It("holds everything behind the drum's tracks under strict", func() {
			sched := run(WipStrict)
			Expect(sched.WipLimit.Value).To(Equal(1), "Delta has one track")
			// One at a time, so the third initiative waits for the first two.
			Expect(scheduledFor(sched, "Away from the drum A").StartWeek).To(Equal(6))
			Expect(scheduledFor(sched, "Away from the drum B").StartWeek).To(Equal(10))
			Expect(scheduledFor(sched, "Away from the drum B").BindingConstraint).To(Equal("wip-limit"))
		})

		It("lets work that never touches the drum flow under drum-gated", func() {
			sched := run(WipDrumGated)
			Expect(sched.WipLimit.Value).To(Equal(1))
			Expect(scheduledFor(sched, "Needs the drum").StartWeek).To(BeZero())
			Expect(scheduledFor(sched, "Away from the drum A").StartWeek).To(BeZero(),
				"it consumes no drum time, so the org limit has nothing to protect from it")
			Expect(scheduledFor(sched, "Away from the drum B").StartWeek).To(BeZero())
		})

		It("applies no org limit at all under off", func() {
			sched := run(WipOff)
			Expect(sched.WipLimit.Value).To(BeZero())
			for _, si := range sched.Initiatives {
				Expect(si.BindingConstraint).NotTo(Equal("wip-limit"), si.Name)
			}
		})

		// An explicit number is orthogonal: it replaces the derived value, and the model
		// still decides who counts against it.
		It("lets an explicit limit set the number while the model sets the rule", func() {
			teams, inits := build()
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipDrumGated,
					MaxConcurrentInitiatives: 3, BufferPct: pctOf(0.25)})
			Expect(sched.WipLimit.Value).To(Equal(3))
			Expect(sched.WipLimit.Derived).To(BeFalse())
			Expect(sched.WipLimit.Model).To(Equal(WipDrumGated))
		})

		// AC 1.1: an unconfigured plan still gets ranks, starts and finishes. Unchosen is
		// reported as a state so the UI can demand a choice without withholding the order.
		It("still schedules when no model is chosen, and says it was not chosen", func() {
			sched := run("")
			Expect(sched.WipLimit.Model).To(Equal(WipUnchosen))
			for _, si := range sched.Initiatives {
				Expect(si.ProposedRank).To(BeNumerically(">", 0), si.Name)
				Expect(si.RawFinishWeek).To(BeNumerically(">", si.StartWeek), si.Name)
			}
			By("reading as strict, so nothing moves for a plan that already had an order")
			strict := run(WipStrict)
			for i := range sched.Initiatives {
				Expect(sched.Initiatives[i].StartWeek).To(Equal(strict.Initiatives[i].StartWeek))
				Expect(sched.Initiatives[i].CommitWeek).To(Equal(strict.Initiatives[i].CommitWeek))
			}
		})

		// "off" switches off one limit, not every limit. Claiming otherwise in the UI
		// while the change-absorption cap still delays work would be a plain lie.
		It("leaves the other org caps alone under off, each naming itself", func() {
			teams := []Team{{Name: "Far", Tracks: 6}}
			var inits []Initiative
			for _, n := range []string{"One", "Two", "Three", "Four"} {
				inits = append(inits, Initiative{Name: n, Work: map[string]TeamWork{"Far": podWork(3)},
					StatedPriority: len(inits) + 1, PriorityLocked: true})
			}
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 40, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipOff,
					MaxStartsPerQuarter: 2, BufferPct: pctOf(0.25)})

			Expect(sched.WipLimit.Value).To(BeZero(), "the org WIP limit is off")
			held := scheduledFor(sched, "Three")
			Expect(held.StartWeek).To(BeNumerically(">", 0),
				"the change-absorption cap is a separate setting the planner asked for")
			Expect(held.BindingConstraint).To(Equal("starts-cap"),
				"and it says which limit held the work, not the one that is switched off")
		})

		It("rejects a model nobody implements rather than guessing", func() {
			sched := run("wishful")
			Expect(sched.WipLimit.Model).To(Equal(WipUnchosen),
				"an unrecognised model is not a licence to invent one")
		})

		// The comparison is of the planner's own plan, which is the point: static help
		// text cannot say what a model costs *here*.
		It("reports what each model would cost for this plan", func() {
			sched := run("")
			Expect(sched.WipModels).To(HaveLen(3))

			by := map[string]WipModelOutcome{}
			for _, o := range sched.WipModels {
				by[o.Model] = o
			}
			Expect(by).To(HaveKey(WipStrict))
			Expect(by).To(HaveKey(WipDrumGated))
			Expect(by).To(HaveKey(WipOff))

			Expect(by[WipStrict].LastCommitWeek).To(BeNumerically(">", by[WipDrumGated].LastCommitWeek),
				"strict holds non-drum work back, so the portfolio ends later")
			Expect(by[WipStrict].PodsIdleAllPeriod).To(BeNumerically(">=", by[WipOff].PodsIdleAllPeriod))
			for _, o := range sched.WipModels {
				Expect(o.Objective).To(BeNumerically(">=", 0))
			}
		})

		It("gives the same comparison whichever model is in force", func() {
			Expect(run(WipStrict).WipModels).To(Equal(run(WipOff).WipModels),
				"the comparison describes the plan, not the current choice")
		})
	})

	Describe("the org WIP limit", func() {
		// Decision 22: derived from the drum, never a shipped number, and always
		// labelled with the pod it came from.
		It("derives from the drum pod's tracks and says which pod that was", func() {
			teams, inits := Demo()
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.10},
				SchedulingParams{PeriodStart: specPeriodStart, BufferPct: pctOf(0.25)})
			Expect(sched.WipLimit.Derived).To(BeTrue())
			Expect(sched.WipLimit.FromPod).To(Equal("Delta"))
			Expect(sched.WipLimit.Value).To(Equal(2))
		})

		// FR-008: the cap exists per pod as well as org-wide, and a pod holding work
		// back for its own concurrency cap is a different remedy from a busy track.
		It("caps concurrent initiatives per pod and reports that separately from a busy track", func() {
			teams := []Team{{Name: "Atlas", Tracks: 4}}
			inits := []Initiative{
				{Name: "One", Work: map[string]TeamWork{"Atlas": podWork(3)}, StatedPriority: 1, PriorityLocked: true},
				{Name: "Two", Work: map[string]TeamWork{"Atlas": podWork(3)}, StatedPriority: 2, PriorityLocked: true},
				{Name: "Three", Work: map[string]TeamWork{"Atlas": podWork(3)}, StatedPriority: 3, PriorityLocked: true},
			}
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 3,
					MaxInitiativesPerPod: 2, BufferPct: pctOf(0.25)})

			// Atlas has 4 free tracks, so only its own cap of 2 can delay the third.
			Expect(podScheduleFor(sched, "Atlas").Weeks[0].Initiatives).To(ConsistOf("One", "Two"))
			third := scheduledFor(sched, "Three")
			Expect(third.StartWeek).To(Equal(3))
			Expect(third.BindingConstraint).To(Equal("pod-wip-limit"),
				"FR-008 wants which limit delayed it, and this is the pod's cap, not the org's")
		})

		It("lets an explicit value win", func() {
			teams, inits := Demo()
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.10},
				SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 5, BufferPct: pctOf(0.25)})
			Expect(sched.WipLimit.Derived).To(BeFalse())
			Expect(sched.WipLimit.Value).To(Equal(5))
		})
	})

	// AC X.2: reuse the existing unknown-pod warning rather than inventing a new one.
	Describe("an initiative needing a pod that is not on the roster", func() {
		It("reports it unschedulable and warns about the pod", func() {
			teams := []Team{{Name: "Atlas", Tracks: 2}}
			inits := []Initiative{{Name: "Needs a ghost", Work: map[string]TeamWork{
				"Atlas": podWork(3), "Phantom": podWork(5, "Atlas"),
			}}}
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, BufferPct: pctOf(0.25)})

			Expect(scheduledFor(sched, "Needs a ghost").Verdict).To(Equal("unschedulable"))
			Expect(strings.Join(sched.Warnings, " ")).To(ContainSubstring("Phantom"))
		})
	})

	Describe("carryover already in flight at the period start", func() {
		// AC X.4: work that is already running cannot be un-started, so the WIP limit
		// must not push it later — but it does occupy a slot, so it holds new work back.
		It("is not held behind the WIP limit, but still counts toward it", func() {
			teams := []Team{{Name: "Atlas", Tracks: 8}} // ample tracks: only WIP can bind
			inits := []Initiative{
				{Name: "New one", Work: map[string]TeamWork{"Atlas": podWork(4)},
					StatedPriority: 1, PriorityLocked: true},
				{Name: "New two", Work: map[string]TeamWork{"Atlas": podWork(4)},
					StatedPriority: 2, PriorityLocked: true},
				{Name: "Carryover", Work: map[string]TeamWork{"Atlas": podWork(10)},
					InFlight: true, ProgressPct: 0.6, StatedPriority: 3, PriorityLocked: true},
			}
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 2, BufferPct: pctOf(0.25)})

			carry := scheduledFor(sched, "Carryover")
			Expect(carry.StartWeek).To(Equal(0), "it is already running; the limit cannot push it later")
			Expect(carry.BindingConstraint).NotTo(Equal("wip-limit"))
			Expect(carry.RawFinishWeek).To(Equal(4), "only the remaining 40% of 10 weeks is left")

			Expect(scheduledFor(sched, "New one").StartWeek).To(Equal(0))
			held := scheduledFor(sched, "New two")
			Expect(held.StartWeek).To(Equal(4), "carryover holds the second slot until it finishes")
			Expect(held.BindingConstraint).To(Equal("wip-limit"))
		})

		It("ranks carryover on the work that is left, not the original estimate", func() {
			teams := []Team{{Name: "Delta", Tracks: 1}}
			inits := []Initiative{{Name: "Nearly done", Work: map[string]TeamWork{"Delta": podWork(10)},
				InFlight: true, ProgressPct: 0.8, CostOfDelayPerWeek: 5}}
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, BufferPct: pctOf(0.25)})
			Expect(scheduledFor(sched, "Nearly done").RankingTerms.ConstraintWeeks).To(Equal(2.0),
				"8 of its 10 weeks are done, so it consumes 2 weeks of the drum, not 10")
		})
	})

	// Ranking moved to residual work, so drum selection has to move with it or the
	// scheduler constrains a pod that is nearly done.
	It("picks the drum from residual work, not the original estimates", func() {
		teams := []Team{{Name: "Heavy", Tracks: 1}, {Name: "Real", Tracks: 1}}
		inits := []Initiative{
			{Name: "Nearly finished", Work: map[string]TeamWork{"Heavy": podWork(20)},
				InFlight: true, ProgressPct: 0.9},
			{Name: "All still ahead", Work: map[string]TeamWork{"Real": podWork(6)}},
		}
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
			SchedulingParams{PeriodStart: specPeriodStart, BufferPct: pctOf(0.25)})

		// Heavy carries 20 weeks on paper but only 2 are left; Real has all 6 to go.
		Expect(sched.DrumPods).To(ConsistOf("Real"))
		Expect(sched.WipLimit.FromPod).To(Equal("Real"))
	})

	It("does not hold carryover behind the full-kit gate either", func() {
		teams := []Team{{Name: "Atlas", Tracks: 4}}
		inits := []Initiative{{Name: "Running already", Work: map[string]TeamWork{"Atlas": podWork(8)},
			InFlight: true, ProgressPct: 0.5, KitPct: 0.1}}
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
			SchedulingParams{PeriodStart: specPeriodStart, KitGate: 0.8, BufferPct: pctOf(0.25)})

		si := scheduledFor(sched, "Running already")
		Expect(si.StartWeek).To(Equal(0), "a readiness gate cannot un-start work that is already running")
		Expect(si.BindingConstraint).NotTo(Equal("kit-gate"))
	})

	// AC X.1 for cross-initiative precedence: a broken edge has to be reported, or
	// the order reads as fully precedence-compliant when it is not.
	It("reports a cycle in afterInitiatives instead of silently dropping an edge", func() {
		teams := []Team{{Name: "Atlas", Tracks: 4}}
		inits := []Initiative{
			{Name: "Chicken", Work: map[string]TeamWork{"Atlas": podWork(3)}, AfterInitiatives: []string{"Egg"}},
			{Name: "Egg", Work: map[string]TeamWork{"Atlas": podWork(3)}, AfterInitiatives: []string{"Chicken"}},
		}
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
			SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 2, BufferPct: pctOf(0.25)})

		joined := strings.Join(sched.Assumptions, " | ")
		Expect(joined).To(ContainSubstring("Chicken"))
		Expect(joined).To(ContainSubstring("Egg"))
		Expect(joined).To(ContainSubstring("cycle"))
	})

	// The edge that gets broken depends on the traversal, so the assumption has to
	// come from the run that was actually returned. Naming the edge a sheet-order
	// walk would have broken points at the wrong initiative.
	It("names the edge the winning order actually broke", func() {
		teams := []Team{{Name: "Atlas", Tracks: 4}}
		inits := []Initiative{
			// Sheet order is Chicken first, but Egg outranks it by delay weight, so the
			// winning rule releases the pair in the opposite order to the sheet.
			{Name: "Chicken", Work: map[string]TeamWork{"Atlas": podWork(3)},
				AfterInitiatives: []string{"Egg"}, Tier: 4, CostOfDelayPerWeek: 1},
			{Name: "Egg", Work: map[string]TeamWork{"Atlas": podWork(3)},
				AfterInitiatives: []string{"Chicken"}, Tier: 1, CostOfDelayPerWeek: 10},
		}
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
			SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 2, BufferPct: pctOf(0.25)})

		chicken := scheduledFor(sched, "Chicken")
		egg := scheduledFor(sched, "Egg")
		Expect(egg.ProposedRank).To(Equal(1), "Egg carries the delay weight")

		// Exactly one of the pair can honour its precedence; the assumption must name
		// whichever one did not.
		Expect(egg.StartWeek).To(BeNumerically(">=", chicken.CommitWeek),
			"Egg waited for Chicken, so Chicken is the edge that was broken")
		Expect(strings.Join(sched.Assumptions, " | ")).To(
			ContainSubstring("Chicken is ordered without waiting for it"))
	})

	// AC 1.4 again: Utilization sorts on rho alone over a map, so equal-rho pods can
	// arrive in either order. The drum must not depend on that.
	It("picks the same drum on every run when two pods are equally loaded", func() {
		teams := []Team{{Name: "Alpha", Tracks: 2}, {Name: "Zulu", Tracks: 2}}
		inits := []Initiative{{Name: "Even", Work: map[string]TeamWork{
			"Alpha": podWork(6), "Zulu": podWork(6),
		}}}
		params := Params{HorizonWeeks: 26, CapacityLoss: 0}
		sp := SchedulingParams{PeriodStart: specPeriodStart, BufferPct: pctOf(0.25)}

		first := ComputeSchedule(teams, inits, params, sp).WipLimit.FromPod
		Expect(first).To(Equal("Alpha"), "ties break on pod name so the choice is reproducible")
		for i := 0; i < 50; i++ {
			Expect(ComputeSchedule(teams, inits, params, sp).WipLimit.FromPod).To(Equal(first))
		}
	})

	// FR-007: a predecessor that ranks lower is still a predecessor.
	It("starts an initiative after its predecessor even when the predecessor ranks lower", func() {
		teams := []Team{{Name: "Atlas", Tracks: 4}}
		inits := []Initiative{
			{Name: "Dependent", Work: map[string]TeamWork{"Atlas": podWork(3)},
				AfterInitiatives: []string{"Prerequisite"}, StatedPriority: 1, PriorityLocked: true},
			{Name: "Prerequisite", Work: map[string]TeamWork{"Atlas": podWork(5)},
				StatedPriority: 2, PriorityLocked: true},
		}
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
			SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 2, BufferPct: pctOf(0.25)})

		pre := scheduledFor(sched, "Prerequisite")
		dep := scheduledFor(sched, "Dependent")
		Expect(dep.ProposedRank).To(Equal(1), "the lock still pins its rank")
		Expect(dep.StartWeek).To(BeNumerically(">=", pre.CommitWeek),
			"released later than its rank, because precedence outranks the dispatch order")
		Expect(dep.BindingConstraint).To(Equal("predecessor"))
	})

	// FR-021: the terms reported must be the terms that produced the position, so
	// the index has to use the same portfolio-average processing time the ranking did.
	It("reports the ranking index that actually produced the order", func() {
		teams := []Team{{Name: "Delta", Tracks: 1}, {Name: "Atlas", Tracks: 2}}
		inits := []Initiative{
			{Name: "Payments GA", Work: map[string]TeamWork{"Atlas": podWork(5), "Delta": podWork(6, "Atlas")},
				Tier: 1, CostOfDelayPerWeek: 9, TargetDate: weekDate(16)},
			{Name: "Search revamp", Work: map[string]TeamWork{"Delta": podWork(2)},
				Tier: 3, CostOfDelayPerWeek: 2},
		}
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
			SchedulingParams{PeriodStart: specPeriodStart, MaxConcurrentInitiatives: 2, BufferPct: pctOf(0.25)})

		// weight 9x4 = 36 over 6 drum weeks = 6.0, discounted by exp(-5/(2*4)) for
		// 5 weeks of slack against a portfolio mean of 4 drum weeks.
		Expect(scheduledFor(sched, "Payments GA").RankingTerms.Index).To(Equal(3.2))
		Expect(scheduledFor(sched, "Search revamp").RankingTerms.Index).To(Equal(2.0),
			"undated work is pure WSJF: weight 2x2 = 4 over 2 drum weeks")
	})

	// §8 lets /schedule take levers, so the lever pass has to carry an initiative's
	// sequencing attributes through untouched. Rebuilding the struct field by field
	// would drop them and the schedule would lose the dates it exists to test.
	Describe("applying levers before scheduling", func() {
		It("keeps every sequencing attribute through the lever pass", func() {
			teams := []Team{{Name: "Atlas", Tracks: 1}}
			inits := []Initiative{{
				Name: "Dated", Work: map[string]TeamWork{"Atlas": podWork(8)},
				StatedPriority: 3, PriorityLocked: true, TargetDate: weekDate(9), DateLocked: true,
				Tier: 1, CostOfDelayPerWeek: 7, EarliestStart: weekDate(1),
				AfterInitiatives: []string{"Something"}, KitPct: 0.8, InFlight: true, ProgressPct: 0.5,
			}}
			_, levered := ApplyLevers(teams, inits, []Lever{{Type: "addCapacity", Pod: "Atlas", N: 1}})
			Expect(levered).To(HaveLen(1))
			kept := levered[0]
			kept.Work = inits[0].Work // the map is deliberately a fresh copy
			Expect(kept).To(Equal(inits[0]))
		})
	})

	// Decision 20 as amended: absent means 25%, an explicit 0 means the planner
	// has chosen to commit on the raw finish.
	Describe("buffer sizing", func() {
		var teams []Team
		var inits []Initiative

		BeforeEach(func() {
			teams = []Team{{Name: "Atlas", Tracks: 2}}
			inits = []Initiative{{Name: "Solo", Work: map[string]TeamWork{"Atlas": podWork(8)}}}
		})

		It("defaults an absent bufferPct to a quarter of the chain", func() {
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart})
			si := scheduledFor(sched, "Solo")
			Expect(si.BufferWeeks).To(Equal(2))
			Expect(si.CommitWeek).To(Equal(10))
		})

		It("honours an explicit zero", func() {
			sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0},
				SchedulingParams{PeriodStart: specPeriodStart, BufferPct: pctOf(0)})
			si := scheduledFor(sched, "Solo")
			Expect(si.BufferWeeks).To(Equal(0))
			Expect(si.CommitWeek).To(Equal(si.RawFinishWeek))
		})
	})

	// Spec 004 Story 1: the deviation carries the pin state so the view can offer
	// pin/unpin without a second lookup. The demo plan's own lock (Access-control,
	// stated-3) never deviates under any winning rule, so exercise reconcile
	// directly with a moved-and-locked row — the case the reason text describes.
	It("carries the priority lock on each rank deviation", func() {
		sis := []ScheduledInitiative{{
			Name: "Pinned", StatedRank: 2, ProposedRank: 5, PriorityLocked: true,
		}, {
			Name: "Free", StatedRank: 5, ProposedRank: 2,
		}}
		devs := reconcile(sis, "constraint-first")
		Expect(devs).To(HaveLen(2))
		byName := map[string]RankDeviation{}
		for _, d := range devs {
			byName[d.Initiative] = d
		}
		Expect(byName["Pinned"].Locked).To(BeTrue())
		Expect(byName["Pinned"].Reason).To(ContainSubstring("locked"))
		Expect(byName["Free"].Locked).To(BeFalse())
	})

	// Spec 004 Story 2 (FR-024): the plan-time fever point, read at the target
	// week. On-time dates burn nothing; a date before the commit consumes buffer
	// in proportion to the miss; no date or no buffer leaves the origin.
	It("computes the fever point against the buffered commit", func() {
		teams, inits := Demo()
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1},
			DemoScheduling())
		byName := map[string]ScheduledInitiative{}
		for _, si := range sched.Initiatives {
			byName[si.Name] = si
		}
		// Telemetry GA lands exactly on its buffered commit: nothing burns.
		onTime := byName["Telemetry GA"]
		Expect(onTime.Verdict).To(Equal("on-time"))
		Expect(onTime.TargetBurn).To(BeNumerically("~", 0, 1e-9))
		// Undated initiatives have no fever point at all.
		undated := byName["SCIM provisioning"]
		Expect(undated.TargetBurn).To(Equal(0.0))
		Expect(undated.TargetProgress).To(Equal(0.0))
	})

	// The late case has its own fixture rather than borrowing one from Demo.
	// Decision 28 stopped the schedule at the horizon, and Demo is WIP-limited
	// enough that six of its ten initiatives now report beyond-horizon and none
	// report late — so the assertion that used to ride on "Managed database MVP"
	// was testing Demo's tuning as much as the fever point. A two-initiative plan
	// that misses a date while staying inside its period cannot drift that way.
	It("computes the fever point for a date missed inside the period", func() {
		teams := []Team{{Name: "Alpha", Tracks: 1}}
		inits := []Initiative{
			{Name: "ahead", Work: map[string]TeamWork{"Alpha": podWork(4)},
				StatedPriority: 1, PriorityLocked: true},
			// Queued behind "ahead", so it commits around week 10 against a week-5
			// target: late by contention, not structurally impossible.
			{Name: "missed", Work: map[string]TeamWork{"Alpha": podWork(4)},
				TargetDate: weekDate(5), StatedPriority: 2, PriorityLocked: true},
		}
		sp := SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipStrict, BufferPct: pctOf(0.25)}
		sched := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26}, sp)

		var late ScheduledInitiative
		for _, si := range sched.Initiatives {
			if si.Name == "missed" {
				late = si
			}
		}
		Expect(late.Verdict).To(Equal("late"), "it starts inside the period, so it has a date verdict")
		Expect(late.CommitWeek).To(BeNumerically(">", 5))
		Expect(late.TargetBurn).To(BeNumerically(">", 1), "missed by more than its buffer")
		// Underway but not finished at the target week. It cannot be zero here and
		// still be "late": a target early enough that the chain has not started is
		// one no ordering could have met, which verdictFor reports as
		// structurally-infeasible instead (Decision 12).
		Expect(late.TargetProgress).To(BeNumerically(">", 0))
		Expect(late.TargetProgress).To(BeNumerically("<", 1))
		// The progress > 0 branch of the ratio, which the old Demo-based assertion
		// never reached: burn per unit of progress, not the raw burn.
		Expect(late.BurnRatio).To(BeNumerically("~", late.TargetBurn/late.TargetProgress, 1e-9))
		Expect(late.BurnRatio).To(BeNumerically(">", late.TargetBurn))
	})

	// Spec 004 Story 5 (AC 5.1/5.2): targetUtilization staggers releases at the
	// drum; absent or 0 leaves the schedule untouched.
	It("staggers drum releases under a target utilization", func() {
		teams, inits := Demo()
		base := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, DemoScheduling())
		sp := DemoScheduling()
		sp.TargetUtilization = 0.8
		staggered := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, sp)
		// Something must have been held, and at least one binding reason must
		// name the stagger — or the knob silently does nothing, which is the
		// failure mode spec 004 exists to close.
		seen := false
		for _, si := range staggered.Initiatives {
			if si.BindingConstraint == "drum stagger" {
				seen = true
				break
			}
		}
		Expect(seen).To(BeTrue(), "some release should be held with reason 'drum stagger'")
		// The demo's drum (Delta) runs at or under the target in every week the
		// stagger controls: its peak weekly load must not exceed 80% of tracks.
		for _, ps := range staggered.PodWeeks {
			if ps.Pod != "Delta" {
				continue
			}
			for _, w := range ps.Weeks[:26] {
				if w.Tracks > 0 {
					Expect(float64(w.Busy)).To(BeNumerically("<=", 0.8*float64(w.Tracks)+1e-9),
						"week %d: drum load %d/%d exceeds the 0.8 target", w.Week, w.Busy, w.Tracks)
				}
			}
		}
		// Sanity: the staggered schedule cannot finish earlier than the base one.
		baseFin := 0
		for _, si := range base.Initiatives {
			if si.CommitWeek > baseFin {
				baseFin = si.CommitWeek
			}
		}
		stagFin := 0
		for _, si := range staggered.Initiatives {
			if si.CommitWeek > stagFin {
				stagFin = si.CommitWeek
			}
		}
		Expect(stagFin).To(BeNumerically(">=", baseFin))
	})

	It("leaves the schedule unchanged with no target utilization", func() {
		teams, inits := Demo()
		off := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, DemoScheduling())
		zero := DemoScheduling()
		zero.TargetUtilization = 0
		zeroed := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, zero)
		Expect(zeroed.ObjectiveScore).To(Equal(off.ObjectiveScore))
		for _, si := range zeroed.Initiatives {
			Expect(si.BindingConstraint).NotTo(Equal("drum stagger"))
		}
	})

})
