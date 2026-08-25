package planning

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BaselineInputs", func() {
	var teams []Team
	var inits []Initiative
	var params Params
	var sp SchedulingParams

	BeforeEach(func() {
		teams, inits = Demo()
		params = Params{HorizonWeeks: 26, CapacityLoss: 0.1}
		sp = DemoScheduling()
	})

	// AC 7.1: "recomputing from the stored inputs reproduces the stored schedule
	// exactly". Stated as a property rather than trusted, because a baseline whose
	// schedule cannot be reproduced cannot answer "why did it say week 4" later.
	It("reproduces the stored schedule exactly", func() {
		in := NewBaselineInputs(teams, inits, params, sp)
		stored := ComputeSchedule(teams, inits, params, sp)

		again := in.Recompute()

		storedJSON, err := json.Marshal(stored)
		Expect(err).NotTo(HaveOccurred())
		againJSON, err := json.Marshal(again)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(againJSON)).To(Equal(string(storedJSON)))
	})

	It("survives a round trip through JSON and still reproduces it", func() {
		in := NewBaselineInputs(teams, inits, params, sp)
		blob, err := json.Marshal(in)
		Expect(err).NotTo(HaveOccurred())

		var back BaselineInputs
		Expect(json.Unmarshal(blob, &back)).To(Succeed())

		want, err := json.Marshal(in.Recompute())
		Expect(err).NotTo(HaveOccurred())
		got, err := json.Marshal(back.Recompute())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(got)).To(Equal(string(want)), "storage must be lossless, or a baseline lies")
	})

	// FR-030: a snapshot that changes underneath is not a snapshot. Every reference
	// the caller could still hold has to be its own copy.
	It("is not disturbed by later edits to what the caller still holds", func() {
		buf := 0.25
		mutableSp := SchedulingParams{PeriodStart: specPeriodStart, BufferPct: &buf,
			LeadCapacity: map[string]int{"pm": 2}}
		mutable := []Initiative{{
			Name: "Shared", Work: map[string]TeamWork{"Delta": podWork(4, "Atlas")},
			Leads: map[string]string{"pm": "Ann"}, AfterInitiatives: []string{"Something"},
		}}
		in := NewBaselineInputs(teams, mutable, params, mutableSp)
		before := in.Fingerprint()

		By("mutating every reference the caller kept")
		mutable[0].Work["Delta"] = TeamWork{Weeks: 99, Estimated: true, InPath: true}
		mutable[0].Work["Sneaky"] = TeamWork{Weeks: 5, Estimated: true, InPath: true}
		mutable[0].Leads["pm"] = "Someone else"
		mutable[0].AfterInitiatives[0] = "Rewritten"
		mutableSp.LeadCapacity["pm"] = 99
		buf = 0.9

		Expect(in.Fingerprint()).To(Equal(before), "the frozen inputs must not have moved")
		Expect(in.Initiatives[0].Work["Delta"].Weeks).To(Equal(4.0))
		Expect(in.Initiatives[0].Work).NotTo(HaveKey("Sneaky"))
		Expect(in.Initiatives[0].Leads["pm"]).To(Equal("Ann"))
		Expect(in.Initiatives[0].AfterInitiatives[0]).To(Equal("Something"))
		Expect(in.Scheduling.LeadCapacity["pm"]).To(Equal(2))
		Expect(*in.Scheduling.BufferPct).To(Equal(0.25))
	})

	It("keeps an explicit zero buffer distinguishable from an absent one", func() {
		zero := 0.0
		in := NewBaselineInputs(teams, inits, params, SchedulingParams{BufferPct: &zero})
		Expect(in.Scheduling.BufferPct).NotTo(BeNil())
		Expect(*in.Scheduling.BufferPct).To(BeZero())
		Expect(NewBaselineInputs(teams, inits, params, SchedulingParams{}).Scheduling.BufferPct).To(BeNil())
	})

	Describe("the fingerprint", func() {
		It("is stable for the same inputs", func() {
			a := NewBaselineInputs(teams, inits, params, sp).Fingerprint()
			b := NewBaselineInputs(teams, inits, params, sp).Fingerprint()
			Expect(a).To(Equal(b))
			Expect(a).NotTo(BeEmpty())
		})

		// Decision 27: everything is fingerprinted, so a new field cannot be
		// forgotten. These cover one input of each kind.
		It("changes when any input changes", func() {
			base := NewBaselineInputs(teams, inits, params, sp).Fingerprint()

			By("a roster edit")
			hotter := append([]Team(nil), teams...)
			hotter[0].Tracks++
			Expect(NewBaselineInputs(hotter, inits, params, sp).Fingerprint()).NotTo(Equal(base))

			By("a sequencing attribute")
			retimed := append([]Initiative(nil), inits...)
			retimed[0].TargetDate = "2026-05-04"
			Expect(NewBaselineInputs(teams, retimed, params, sp).Fingerprint()).NotTo(Equal(base))

			By("an estimate")
			rescoped := append([]Initiative(nil), inits...)
			rescoped[1].Work = map[string]TeamWork{"Delta": {Weeks: 99, Estimated: true, InPath: true}}
			Expect(NewBaselineInputs(teams, rescoped, params, sp).Fingerprint()).NotTo(Equal(base))

			By("the horizon")
			longer := params
			longer.HorizonWeeks = 40
			Expect(NewBaselineInputs(teams, inits, longer, sp).Fingerprint()).NotTo(Equal(base))

			By("a scheduling parameter")
			gated := sp
			gated.WipModel = WipDrumGated
			Expect(NewBaselineInputs(teams, inits, params, gated).Fingerprint()).NotTo(Equal(base))
		})

		// The conservative direction Decision 27 chose on purpose: a cosmetic edit
		// flags divergence, because the alternative is a real change going unnoticed.
		It("changes even for an edit that cannot affect the schedule", func() {
			base := NewBaselineInputs(teams, inits, params, sp).Fingerprint()
			retitled := append([]Initiative(nil), inits...)
			retitled[0].Description = "reworded, same work"
			Expect(NewBaselineInputs(teams, retitled, params, sp).Fingerprint()).NotTo(Equal(base),
				"a false positive is visible; a missed change is not")
		})

		It("does not depend on map iteration order", func() {
			// Same work, built in a different insertion order: Go randomises map
			// iteration, so an implementation that walked the map would be unstable.
			one := []Initiative{{Name: "A", Work: map[string]TeamWork{
				"Delta": {Weeks: 3, Estimated: true, InPath: true},
				"Atlas": {Weeks: 4, Estimated: true, InPath: true},
			}}}
			two := []Initiative{{Name: "A", Work: map[string]TeamWork{
				"Atlas": {Weeks: 4, Estimated: true, InPath: true},
				"Delta": {Weeks: 3, Estimated: true, InPath: true},
			}}}
			for i := 0; i < 20; i++ {
				Expect(NewBaselineInputs(teams, two, params, sp).Fingerprint()).
					To(Equal(NewBaselineInputs(teams, one, params, sp).Fingerprint()))
			}
		})
	})
})

var _ = Describe("CompareToBaseline", func() {
	var teams []Team
	var inits []Initiative
	var params Params
	var sp SchedulingParams

	BeforeEach(func() {
		teams = []Team{{Name: "Delta", Tracks: 1}}
		inits = []Initiative{
			{Name: "First", Work: map[string]TeamWork{"Delta": podWork(4)}, StatedPriority: 1, PriorityLocked: true},
			{Name: "Second", Work: map[string]TeamWork{"Delta": podWork(4)}, StatedPriority: 2, PriorityLocked: true},
		}
		params = Params{HorizonWeeks: 26, CapacityLoss: 0}
		sp = SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipStrict, BufferPct: pctOf(0.25)}
	})

	It("reports no movement when nothing changed", func() {
		base := ComputeSchedule(teams, inits, params, sp)
		cmp := CompareToBaseline(base, ComputeSchedule(teams, inits, params, sp))

		Expect(cmp.Added).To(BeEmpty())
		Expect(cmp.Removed).To(BeEmpty())
		Expect(cmp.Moved).To(BeZero())
		for _, d := range cmp.Initiatives {
			Expect(d.StartDeltaWeeks).To(BeZero(), d.Name)
			Expect(d.CommitDeltaWeeks).To(BeZero(), d.Name)
		}
	})

	// AC 7.4: per-initiative start and commit deltas in weeks.
	It("reports the start and commit deltas in weeks", func() {
		base := ComputeSchedule(teams, inits, params, sp)

		slower := append([]Initiative(nil), inits...)
		slower[0].Work = map[string]TeamWork{"Delta": podWork(6)} // First takes 2 weeks longer
		cmp := CompareToBaseline(base, ComputeSchedule(teams, slower, params, sp))

		byName := map[string]BaselineDelta{}
		for _, d := range cmp.Initiatives {
			byName[d.Name] = d
		}
		Expect(byName["First"].CommitDeltaWeeks).To(Equal(3), "4w -> 6w, plus a week of buffer")
		Expect(byName["Second"].StartDeltaWeeks).To(Equal(2), "it waits two weeks longer for the track")
		Expect(cmp.Moved).To(Equal(2))
	})

	// AC 7.4: added and removed listed separately, not as movement.
	It("lists initiatives added and removed since the baseline", func() {
		base := ComputeSchedule(teams, inits, params, sp)

		changed := []Initiative{
			inits[0],
			{Name: "Third", Work: map[string]TeamWork{"Delta": podWork(2)}, StatedPriority: 3, PriorityLocked: true},
		}
		cmp := CompareToBaseline(base, ComputeSchedule(teams, changed, params, sp))

		Expect(cmp.Added).To(ConsistOf("Third"))
		Expect(cmp.Removed).To(ConsistOf("Second"))
		for _, d := range cmp.Initiatives {
			Expect(d.Name).NotTo(Equal("Third"), "an addition has no baseline to be a delta from")
			Expect(d.Name).NotTo(Equal("Second"))
		}
	})

	It("reports a verdict that changed, since that is the part people act on", func() {
		dated := append([]Initiative(nil), inits...)
		dated[1].TargetDate = weekDate(9)
		base := ComputeSchedule(teams, dated, params, sp)

		slower := append([]Initiative(nil), dated...)
		slower[0].Work = map[string]TeamWork{"Delta": podWork(8)}
		cmp := CompareToBaseline(base, ComputeSchedule(teams, slower, params, sp))

		byName := map[string]BaselineDelta{}
		for _, d := range cmp.Initiatives {
			byName[d.Name] = d
		}
		Expect(byName["Second"].BaselineVerdict).To(Equal("on-time"))
		Expect(byName["Second"].Verdict).To(Equal("late"))
		Expect(byName["Second"].VerdictChanged).To(BeTrue())
	})

	It("survives a nil baseline or a nil current schedule", func() {
		Expect(func() { CompareToBaseline(nil, nil) }).NotTo(Panic())
		cmp := CompareToBaseline(nil, ComputeSchedule(teams, inits, params, sp))
		Expect(cmp.Added).To(ConsistOf("First", "Second"), "everything is new against nothing")
	})

	It("orders the deltas by the current proposed rank, so the table reads like the order", func() {
		// Load-bearing: two initiatives whose ranks flip between the two schedules.
		// An identical-input compare arrives already sorted, so it would pass even
		// if sortDeltasByRank were deleted; this pair would not.
		base := ComputeSchedule(teams, inits, params, sp)

		flipped := append([]Initiative(nil), inits...)
		// Second's work shrinks, so the capacity-feasible order puts it first.
		flipped[1].Work = map[string]TeamWork{"Delta": podWork(1)}
		// Unlocked so the scheduler is free to reorder (locked priorities would
		// pin the stated order and defeat the point of the mutation).
		flipped[0].PriorityLocked = false
		flipped[1].PriorityLocked = false
		current := ComputeSchedule(teams, flipped, params, sp)

		cmp := CompareToBaseline(base, current)
		Expect(cmp.Initiatives).To(HaveLen(2))
		// The two initiatives must actually have swapped, or this spec says nothing
		// about ordering either.
		rank := map[string]int{}
		for _, d := range cmp.Initiatives {
			rank[d.Name] = d.ProposedRank
		}
		Expect(rank["First"]).NotTo(Equal(rank["Second"]))
		for i := 1; i < len(cmp.Initiatives); i++ {
			Expect(cmp.Initiatives[i].ProposedRank).To(BeNumerically(">", cmp.Initiatives[i-1].ProposedRank),
				"delta %s is out of rank order", cmp.Initiatives[i].Name)
		}
	})
})

// Spec 005: baseline-to-baseline comparison uses the same engine and shape as
// the live compare, so both flavours report identical deltas.
var _ = Describe("baseline-to-baseline comparison", func() {
	It("reports the delta between two stored schedules", func() {
		teams, inits := Demo()
		base := ComputeSchedule(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1},
			DemoScheduling())

		// Pin the stated-#1 and recompute: locking the top makes the
		// stated-priority rule win outright (its schedule becomes best), so
		// v2's order follows the stated ranks — the demo's reconciliation
		// fixture shows exactly this flip.
		var changed []Initiative
		for _, it := range inits {
			cp := it
			if cp.StatedPriority == 1 {
				cp.PriorityLocked = true
			}
			changed = append(changed, cp)
		}
		next := ComputeSchedule(teams, changed, Params{HorizonWeeks: 26, CapacityLoss: 0.1},
			DemoScheduling())

		cmp := CompareToBaseline(base, next)
		Expect(cmp.Moved).To(BeNumerically(">", 0))
		Expect(cmp.Initiatives).NotTo(BeEmpty())
		// Same shape as the live compare: every initiative present, with the
		// baseline side carrying its own rank.
		byName := map[string]BaselineDelta{}
		for _, d := range cmp.Initiatives {
			byName[d.Name] = d
		}
		Expect(byName).To(HaveLen(len(next.Initiatives)))
		// The stated-#2 (Telemetry GA) held rank 1 in v1's constraint-first
		// order but follows its stated rank under v2's stated-priority run.
		Expect(byName["Telemetry GA"].BaselineRank).To(Equal(1))
		Expect(byName["Telemetry GA"].ProposedRank).To(Equal(2))
	})
})
