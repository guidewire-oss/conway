package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApplyInitiativeEdits", func() {
	var inits []Initiative
	var params SchedulingParams

	BeforeEach(func() {
		inits = []Initiative{
			{
				Name:        "Payments GA",
				Description: "keep me",
				Leads:       map[string]string{"pm": "Ann"},
				Work: map[string]TeamWork{
					"Delta": {Weeks: 6, Estimated: true, InPath: true},
				},
				StatedPriority: 2, TargetDate: "2026-03-30", Tier: 1, KitPct: 0.5,
			},
			{
				Name: "Search revamp",
				Work: map[string]TeamWork{"Atlas": {Weeks: 3, Estimated: true, InPath: true}},
			},
		}
		params = SchedulingParams{PeriodStart: "2026-01-05"}
	})

	// The whole point of pointer fields: a UI that edits one cell must not clear the
	// rest of the row just by not mentioning it.
	It("changes only the attributes the edit mentions", func() {
		pri := 1
		out, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
			{Name: "Payments GA", StatedPriority: &pri},
		}, params, 26)
		Expect(err).NotTo(HaveOccurred())

		got := out[0]
		Expect(got.StatedPriority).To(Equal(1))
		Expect(got.TargetDate).To(Equal("2026-03-30"), "an unmentioned attribute must survive")
		Expect(got.Tier).To(Equal(1))
		Expect(got.KitPct).To(Equal(0.5))
	})

	It("clears an attribute when the edit says so explicitly", func() {
		empty := ""
		zero := 0
		out, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
			{Name: "Payments GA", TargetDate: &empty, StatedPriority: &zero},
		}, params, 26)
		Expect(err).NotTo(HaveOccurred())
		Expect(out[0].TargetDate).To(BeEmpty())
		Expect(out[0].StatedPriority).To(BeZero())
	})

	It("leaves the name, description, leads and work alone", func() {
		locked := true
		out, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
			{Name: "Payments GA", PriorityLocked: &locked},
		}, params, 26)
		Expect(err).NotTo(HaveOccurred())

		Expect(out[0].Name).To(Equal("Payments GA"))
		Expect(out[0].Description).To(Equal("keep me"))
		Expect(out[0].Leads).To(HaveKeyWithValue("pm", "Ann"))
		Expect(out[0].Work).To(HaveKey("Delta"))
		Expect(out[0].Work["Delta"].Weeks).To(Equal(6.0))
	})

	It("does not touch the initiatives it was not asked about", func() {
		pri := 1
		out, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
			{Name: "Payments GA", StatedPriority: &pri},
		}, params, 26)
		Expect(err).NotTo(HaveOccurred())
		Expect(out[1]).To(Equal(inits[1]))
	})

	It("leaves the caller's slice untouched", func() {
		pri := 1
		_, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
			{Name: "Payments GA", StatedPriority: &pri},
		}, params, 26)
		Expect(err).NotTo(HaveOccurred())
		Expect(inits[0].StatedPriority).To(Equal(2), "the input must not be mutated in place")
	})

	It("rejects an edit naming an initiative the plan does not have", func() {
		pri := 1
		_, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
			{Name: "Not in this plan", StatedPriority: &pri},
		}, params, 26)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Not in this plan"))
	})

	It("applies several edits in one call", func() {
		pri1, pri2 := 1, 2
		out, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
			{Name: "Search revamp", StatedPriority: &pri1},
			{Name: "Payments GA", StatedPriority: &pri2},
		}, params, 26)
		Expect(err).NotTo(HaveOccurred())
		Expect(out[0].StatedPriority).To(Equal(2))
		Expect(out[1].StatedPriority).To(Equal(1))
	})

	Describe("date validation (AC 2.4)", func() {
		It("rejects a target date before the period starts, naming the bounds", func() {
			early := "2025-12-01"
			_, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
				{Name: "Payments GA", TargetDate: &early},
			}, params, 26)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("2025-12-01"))
			Expect(err.Error()).To(ContainSubstring("2026-01-05"), "the message names the period start")
			Expect(err.Error()).To(ContainSubstring("2026-07-06"), "and the period end")
		})

		It("rejects a target date past the horizon", func() {
			late := "2027-01-04"
			_, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
				{Name: "Payments GA", TargetDate: &late},
			}, params, 26)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("2027-01-04"))
		})

		It("accepts a date on either boundary", func() {
			for _, d := range []string{"2026-01-05", "2026-07-06"} {
				day := d
				_, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
					{Name: "Payments GA", TargetDate: &day},
				}, params, 26)
				Expect(err).NotTo(HaveOccurred(), "boundary date %s must be allowed", day)
			}
		})

		It("rejects a date it cannot read rather than storing it", func() {
			junk := "next Tuesday"
			_, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
				{Name: "Payments GA", TargetDate: &junk},
			}, params, 26)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("next Tuesday"))
		})

		It("normalises an accepted date to ISO", func() {
			written := "30-Mar-2026"
			out, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
				{Name: "Payments GA", TargetDate: &written},
			}, params, 26)
			Expect(err).NotTo(HaveOccurred())
			Expect(out[0].TargetDate).To(Equal("2026-03-30"))
		})

		// Nothing can be measured against a period that has no start, and inventing one
		// would reject dates on a rule the planner never set.
		It("skips the bounds check when the plan has no period start", func() {
			early := "2020-01-01"
			out, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
				{Name: "Payments GA", TargetDate: &early},
			}, SchedulingParams{}, 26)
			Expect(err).NotTo(HaveOccurred())
			Expect(out[0].TargetDate).To(Equal("2020-01-01"))
		})

		It("reports every bad date at once rather than one per round trip", func() {
			early, late := "2025-12-01", "2027-01-04"
			_, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
				{Name: "Payments GA", TargetDate: &early},
				{Name: "Search revamp", TargetDate: &late},
			}, params, 26)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Payments GA"))
			Expect(err.Error()).To(ContainSubstring("Search revamp"))
		})

		// AC 2.4: "and no schedule is recomputed" — nothing may be applied when any
		// value is rejected, or a partial save would leave the plan half-edited.
		It("applies nothing at all when any edit is rejected", func() {
			pri := 1
			bad := "2025-12-01"
			out, err := ApplyInitiativeEdits(inits, []InitiativeEdit{
				{Name: "Search revamp", StatedPriority: &pri},
				{Name: "Payments GA", TargetDate: &bad},
			}, params, 26)
			Expect(err).To(HaveOccurred())
			Expect(out).To(BeNil(), "a rejected batch must not return a partly-edited plan")
			Expect(inits[1].StatedPriority).To(BeZero())
		})
	})
})

var _ = Describe("PeriodBounds", func() {
	It("runs from the period start to the end of the horizon", func() {
		start, end, ok := PeriodBounds(SchedulingParams{PeriodStart: "2026-01-05"}, 26)
		Expect(ok).To(BeTrue())
		Expect(start).To(Equal("2026-01-05"))
		Expect(end).To(Equal("2026-07-06"), "26 weeks after the start")
	})

	It("reports no bounds without a period start", func() {
		_, _, ok := PeriodBounds(SchedulingParams{}, 26)
		Expect(ok).To(BeFalse())
	})

	It("falls back to the default horizon when none is given", func() {
		_, end, ok := PeriodBounds(SchedulingParams{PeriodStart: "2026-01-05"}, 0)
		Expect(ok).To(BeTrue())
		Expect(end).To(Equal("2026-07-06"), "Params.WithDefaults uses 26 weeks")
	})
})
