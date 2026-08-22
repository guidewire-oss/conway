package planning

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Story 5 (AC 5.1-5.5) and AC 3.3. The fixture is small enough to reason about
// by hand: Delta is the 2-track drum, Atlas has slack, and one dated initiative
// misses its date by a known number of weeks.
var _ = Describe("ComputeRemedies", func() {
	var (
		teams []Team
		inits []Initiative
		sp    SchedulingParams
	)

	weekDate := func(w int) string {
		t0, _ := time.Parse("2006-01-02", specPeriodStart)
		return t0.AddDate(0, 0, w*7).Format("2006-01-02")
	}

	BeforeEach(func() {
		teams = []Team{
			{Name: "Delta", Devs: 2, Tracks: 1}, // single-track drum: slices queue
			{Name: "Atlas", Devs: 12, Tracks: 6},
		}
		inits = []Initiative{
			// Unlocked with a tight date: minimum-slack promotes it, so it runs
			// first and holds its date. It is also the victim a raise displaces.
			{Name: "Early", Work: map[string]TeamWork{"Delta": podWork(10)},
				StatedPriority: 3, TargetDate: weekDate(14)},
			// Locked behind Early on the single-track drum: commits at w23
			// against a w20 target, three weeks late. This is the rescue target.
			{Name: "Late", Work: map[string]TeamWork{"Delta": podWork(10)},
				StatedPriority: 2, PriorityLocked: true, TargetDate: weekDate(20)},
			// Slack-filler so the portfolio has a third member.
			{Name: "Other", Work: map[string]TeamWork{"Atlas": podWork(4)},
				StatedPriority: 4},
		}
		sp = SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipStrict,
			BufferPct: pctOf(0.25), MaxConcurrentInitiatives: 3}
	})

	base := func() *Schedule {
		return ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
	}

	It("finds the fixture's miss to begin with", func() {
		// A guard for the fixture itself: if this fails, every spec below is
		// testing the wrong thing.
		s := base()
		byName := map[string]ScheduledInitiative{}
		for _, si := range s.Initiatives {
			byName[si.Name] = si
		}
		Expect(byName["Late"].Verdict).To(Equal(verdictLate),
			"the fixture must produce a late date for remedies to rescue")
		Expect(byName["Late"].WeeksLate).To(BeNumerically(">", 0))
	})

	Describe("AC 5.1: options are proposed and priced", func() {
		It("returns more than one kind, each with a verdict and an objective delta", func() {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)

			kinds := map[string]bool{}
			for _, r := range remedies {
				Expect(r.ResultingVerdict).NotTo(BeEmpty(), "kind %s has no recomputed verdict", r.Kind)
				Expect(r.Target).To(Equal("Late"))
				kinds[r.Kind] = true
			}
			Expect(kinds).NotTo(BeEmpty())
			Expect(len(kinds)).To(BeNumerically(">", 1),
				"FR-015 wants at least raise-priority, descope, add-capacity, relax-date and defer-other in the mix")
		})

		It("orders the list cheapest-first by what the remedy does to the whole portfolio", func() {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)
			Expect(remedies).NotTo(BeEmpty())
			for i := 1; i < len(remedies); i++ {
				Expect(remedies[i].ObjectiveDelta).To(BeNumerically(">=", remedies[i-1].ObjectiveDelta),
					"remedy %d (%s) is cheaper than the one before it", i, remedies[i].Kind)
			}
		})

		It("prices the portfolio effect, not just the target: victims carry week deltas", func() {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)
			for _, r := range remedies {
				for _, v := range r.AffectedInitiatives {
					Expect(v.Initiative).NotTo(Equal("Late"),
						"the target's own movement is reported on the remedy, not as its own victim")
					Expect(v.CommitDeltaWeeks).NotTo(Equal(0),
						"%s lists %s as affected with no week delta", r.Kind, v.Initiative)
				}
			}
		})
	})

	Describe("AC 5.2: raising priority states the priority and names the victims", func() {
		It("states the priority that lands the date and who pays for it", func() {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)

			var raise *Remedy
			for i, r := range remedies {
				if r.Kind == "raise-priority" {
					raise = &remedies[i]
				}
			}
			Expect(raise).NotTo(BeNil(), "the fixture's miss is contention on the drum; an earlier start rescues it")
			Expect(raise.ResultingVerdict).To(Equal(verdictOnTime))
			Expect(int(raise.Magnitude)).To(BeNumerically(">=", 1))
			Expect(int(raise.Magnitude)).To(BeNumerically("<", 2),
				"priority 2 is the fixture's own stated priority; a raise must be strictly earlier")

			// Early holds the drum from w0; promoting Late past it pushes
			// Early out, and the remedy must say so with a number.
			victims := map[string]int{}
			for _, v := range raise.AffectedInitiatives {
				victims[v.Initiative] = v.CommitDeltaWeeks
			}
			Expect(victims).To(HaveKey("Early"), "Early is the initiative the raise displaces")
			Expect(victims["Early"]).To(BeNumerically(">", 0))
		})
	})

	Describe("the remedy kinds", func() {
		It("offers a descope with the fraction that fixes the date", func() {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)
			var descope *Remedy
			for i, r := range remedies {
				if r.Kind == "descope" {
					descope = &remedies[i]
				}
			}
			Expect(descope).NotTo(BeNil())
			Expect(descope.Magnitude).To(BeNumerically(">", 0))
			Expect(descope.Magnitude).To(BeNumerically("<", 1))
		})

		It("offers add-capacity at the pod that binds, with tracks as the magnitude", func() {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)
			var add *Remedy
			for i, r := range remedies {
				if r.Kind == "add-capacity" {
					add = &remedies[i]
				}
			}
			Expect(add).NotTo(BeNil())
			Expect(add.Pod).To(Equal("Delta"), "Delta is the binding pod in this fixture")
			Expect(add.Magnitude).To(BeNumerically(">=", 1))
		})

		It("offers relax-date with the weeks the date has to move, landing on time", func() {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)
			var relax *Remedy
			for i, r := range remedies {
				if r.Kind == "relax-date" {
					relax = &remedies[i]
				}
			}
			Expect(relax).NotTo(BeNil())
			Expect(relax.ResultingVerdict).To(Equal(verdictOnTime))
			Expect(relax.Magnitude).To(BeNumerically(">", 0))
			Expect(relax.Magnitude).To(BeNumerically("<=", 26), "a relaxation past the horizon is not a rescue")
		})

		It("offers defer-other naming the initiative to defer, when deferring it helps", func() {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)
			var deferOther *Remedy
			for i, r := range remedies {
				if r.Kind == "defer-other" {
					deferOther = &remedies[i]
				}
			}
			Expect(deferOther).NotTo(BeNil(),
				"deferring Early frees the drum for Late, so the option must exist")
			Expect(deferOther.ResultingVerdict).To(Equal(verdictOnTime))
		})
	})

	Describe("AC 5.5 / FR-022: computing remedies changes nothing", func() {
		It("leaves the inputs it was given untouched", func() {
			before, err := json.Marshal([]any{teams, inits})
			Expect(err).NotTo(HaveOccurred())

			ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)

			after, err := json.Marshal([]any{teams, inits})
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(MatchJSON(before))
		})
	})

	Describe("targeting", func() {
		It("defaults to every missed date, and honours an explicit target list", func() {
			all := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, nil)
			Expect(all).NotTo(BeEmpty())

			none := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp,
				[]string{"Early", "Other"})
			Expect(none).To(BeEmpty(), "neither Early nor Other misses a date, so there is nothing to rescue")

			one := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, []string{"Late"})
			Expect(one).NotTo(BeEmpty())
			for _, r := range one {
				Expect(r.Target).To(Equal("Late"))
			}
		})
	})

	Describe("NFR-003: remedy generation stays interactive", func() {
		It("prices the full remedy set for the demo plan in under 5 seconds", func() {
			dTeams, dInits := Demo()
			started := time.Now()
			remedies := ComputeRemedies(dTeams, dInits,
				Params{HorizonWeeks: 26, CapacityLoss: 0.1},
				DemoScheduling(), nil)
			elapsed := time.Since(started)
			Expect(remedies).NotTo(BeEmpty(), "the demo plan misses dates, so it must have remedies")
			Expect(elapsed.Seconds()).To(BeNumerically("<", 5),
				"NFR-003: full remedy set in under 5s; took %.1fs", elapsed.Seconds())
		})
	})
})

// AC 3.3: two locks that cannot both hold. The pair must be surfaced on the
// schedule, and each side must come with at least one offered relaxation.
var _ = Describe("conflicting commitments", func() {
	var (
		teams []Team
		inits []Initiative
		sp    SchedulingParams
	)

	weekDate := func(w int) string {
		t0, _ := time.Parse("2006-01-02", specPeriodStart)
		return t0.AddDate(0, 0, w*7).Format("2006-01-02")
	}

	BeforeEach(func() {
		// Two date-locked initiatives queued on a single-track pod: whichever
		// runs second cannot hold its date, and the first's own date is tight
		// enough that no ordering serves both. Neither lock may be quietly
		// broken to escape the clash.
		teams = []Team{{Name: "Delta", Devs: 2, Tracks: 1}}
		inits = []Initiative{
			{Name: "First", Work: map[string]TeamWork{"Delta": podWork(10)},
				StatedPriority: 1, PriorityLocked: true, DateLocked: true, TargetDate: weekDate(12)},
			{Name: "Second", Work: map[string]TeamWork{"Delta": podWork(10)},
				StatedPriority: 2, PriorityLocked: true, DateLocked: true, TargetDate: weekDate(14)},
		}
		sp = SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipStrict, BufferPct: pctOf(0.25)}
	})

	It("reports the pair on the schedule, on the pod they contend for", func() {
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		Expect(s.Conflicts).To(HaveLen(1))
		pair := s.Conflicts[0]
		names := []string{pair.A, pair.B}
		Expect(names).To(ConsistOf("First", "Second"))
		Expect(pair.Pod).To(Equal("Delta"))
	})

	It("violates neither lock silently: both sides still carry their locks and verdicts", func() {
		s := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp)
		byName := map[string]ScheduledInitiative{}
		for _, si := range s.Initiatives {
			byName[si.Name] = si
		}
		Expect(byName["First"].DateLocked).To(BeTrue())
		Expect(byName["Second"].DateLocked).To(BeTrue())
	})

	It("offers at least one relaxation for each side of the conflict", func() {
		relaxations := map[string]bool{"unlock": true, "descope": true, "add-capacity": true, "relax-date": true, "transfer-capacity": true}
		for _, side := range []string{"First", "Second"} {
			remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, []string{side})
			offered := false
			for _, r := range remedies {
				if relaxations[r.Kind] {
					offered = true
				}
			}
			Expect(offered).To(BeTrue(),
				"AC 3.3: %s must come with at least one offered relaxation", side)
		}
	})

	It("offers unlock for a locked commitment that cannot hold", func() {
		remedies := ComputeRemedies(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0}, sp, []string{"Second"})
		kinds := map[string]bool{}
		for _, r := range remedies {
			kinds[r.Kind] = true
		}
		Expect(kinds).To(HaveKey("unlock"))
	})
})
