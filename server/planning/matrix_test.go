package planning

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The optional sequencing columns from spec 001 §8, recognised by header name and
// placed to the left of the full-kit total. §8 also says unrecognised columns are
// ignored, as they always were.
var _ = Describe("ParseMatrix with the sequencing columns", func() {
	// Deliberately awkward: mixed spelling, mixed case, a column §8 does not name,
	// and the team columns after the full-kit total where they have always been.
	header := []string{
		"S.No", "Initiative", "PM Lead",
		"Priority", "Priority Fixed", "Target Date", "Date Fixed", "Tier",
		"Cost of Delay", "Earliest Start", "Depends on Initiative",
		"Kit %", "In Flight", "% Complete", "Notes for the PMO",
		"Full Kit Estimate total", "Alpha Sequence", "Alpha",
	}

	It("reads every attribute §8 names", func() {
		plan := ParseMatrix([][]string{header, {
			"1", "Payments GA", "Ann",
			"2", "yes", "2026-03-30", "y", "1",
			"8", "2026-01-19", "Platform base, Card rails",
			"0.75", "yes", "0.4", "ignore me",
			"12", "NONE", "6",
		}}, nil, false)

		Expect(plan.Initiatives).To(HaveLen(1))
		it := plan.Initiatives[0]
		Expect(it.Name).To(Equal("Payments GA"))
		Expect(it.StatedPriority).To(Equal(2))
		Expect(it.PriorityLocked).To(BeTrue())
		Expect(it.TargetDate).To(Equal("2026-03-30"))
		Expect(it.DateLocked).To(BeTrue())
		Expect(it.Tier).To(Equal(1))
		Expect(it.CostOfDelayPerWeek).To(Equal(8.0))
		Expect(it.EarliestStart).To(Equal("2026-01-19"))
		Expect(it.AfterInitiatives).To(Equal([]string{"Platform base", "Card rails"}))
		Expect(it.KitPct).To(Equal(0.75))
		Expect(it.InFlight).To(BeTrue())
		Expect(it.ProgressPct).To(Equal(0.4))

		By("still reading the leads and team work it always did")
		Expect(it.Leads).To(HaveKeyWithValue("pm", "Ann"))
		Expect(it.Work["Alpha"].Weeks).To(Equal(6.0))
		Expect(it.Work["Alpha"].InPath).To(BeTrue())
	})

	// ReadXLSX renders numbers as their text and never looks at a cell's number
	// format, so a cell a planner formatted as a Date arrives as its serial.
	It("reads a date typed as text or left as an Excel serial number", func() {
		plan := ParseMatrix([][]string{
			{"Initiative", "Target Date", "Earliest Start", "Full Kit Estimate total", "Alpha"},
			{"From a serial", "46111", "46041", "6", "6"},
			{"From text", "2026-03-30", "2026-01-19", "6", "6"},
		}, nil, false)

		Expect(plan.Initiatives).To(HaveLen(2))
		Expect(plan.Initiatives[0].TargetDate).To(Equal("2026-03-30"))
		Expect(plan.Initiatives[0].EarliestStart).To(Equal("2026-01-19"))
		Expect(plan.Initiatives[1].TargetDate).To(Equal("2026-03-30"))
		Expect(plan.Initiatives[1].EarliestStart).To(Equal("2026-01-19"))
	})

	It("accepts a percentage written as a fraction, a number or with a sign", func() {
		plan := ParseMatrix([][]string{
			{"Initiative", "Kit %", "% Complete", "Full Kit Estimate total", "Alpha"},
			{"Fraction", "0.6", "0.25", "6", "6"},
			{"Whole number", "60", "25", "6", "6"},
			{"With a sign", "60%", "25%", "6", "6"},
		}, nil, false)

		for _, it := range plan.Initiatives {
			Expect(it.KitPct).To(Equal(0.6), it.Name)
			Expect(it.ProgressPct).To(Equal(0.25), it.Name)
		}
	})

	// FR-002: a sheet with none of these columns must parse exactly as it did before.
	It("leaves every attribute unset when the sheet has none of the columns", func() {
		plan := ParseMatrix([][]string{
			{"S.No", "Initiative", "PM Lead", "Full Kit Estimate total", "Alpha Sequence", "Alpha"},
			{"1", "Plain old plan", "Ann", "6", "NONE", "6"},
		}, nil, false)

		Expect(plan.Initiatives).To(HaveLen(1))
		it := plan.Initiatives[0]
		Expect(it.StatedPriority).To(BeZero())
		Expect(it.PriorityLocked).To(BeFalse())
		Expect(it.TargetDate).To(BeEmpty())
		Expect(it.DateLocked).To(BeFalse())
		Expect(it.Tier).To(BeZero())
		Expect(it.CostOfDelayPerWeek).To(BeZero())
		Expect(it.EarliestStart).To(BeEmpty())
		Expect(it.AfterInitiatives).To(BeEmpty())
		Expect(it.KitPct).To(BeZero())
		Expect(it.InFlight).To(BeFalse())
		Expect(it.ProgressPct).To(BeZero())
		Expect(it.Work["Alpha"].Weeks).To(Equal(6.0), "the team columns still parse")
	})

	// The bound is the full-kit total column, not the first team column, and the two
	// differ by exactly that column — which can itself look like an attribute when a
	// sheet names its derived total "Full Kit % Estimate".
	It("does not read the full-kit total column as an attribute", func() {
		plan := ParseMatrix([][]string{
			{"Initiative", "Full Kit % Estimate total", "Alpha"},
			{"Percent-named total", "12", "6"},
		}, nil, false)

		Expect(plan.Initiatives[0].KitPct).To(BeZero(),
			"that column is the derived total in weeks, not a readiness percentage")
	})

	// A bare small number in a date column is a typo, not 1900. Reading it as a date
	// would turn a mistyped cell into a confident, wildly wrong verdict.
	It("does not read a small number in a date column as a 1900 date", func() {
		plan := ParseMatrix([][]string{
			{"Initiative", "Target Date", "Earliest Start", "Full Kit Estimate total", "Alpha"},
			{"Typo in the date", "5", "12", "6", "6"},
		}, nil, false)

		it := plan.Initiatives[0]
		Expect(it.TargetDate).To(BeEmpty())
		Expect(it.EarliestStart).To(BeEmpty())
	})

	// The header scan matches on substrings, and "Depends on Initiative" contains
	// "initiative". Whichever column comes first must not be able to claim the name.
	It("does not mistake the depends-on column for the initiative name column", func() {
		plan := ParseMatrix([][]string{
			{"Depends on Initiative", "Initiative", "Full Kit Estimate total", "Alpha"},
			{"Platform base", "Real name", "6", "6"},
		}, nil, false)

		Expect(plan.Initiatives).To(HaveLen(1))
		Expect(plan.Initiatives[0].Name).To(Equal("Real name"))
		Expect(plan.Initiatives[0].AfterInitiatives).To(Equal([]string{"Platform base"}))
	})

	// encoding/json cannot marshal NaN or Inf, so letting one into a field turns the
	// upload response into a dropped request — the same hazard InfiniteRho exists for.
	It("refuses a non-finite number rather than poisoning the response", func() {
		plan := ParseMatrix([][]string{
			{"Initiative", "Kit %", "% Complete", "Cost of Delay", "Full Kit Estimate total", "Alpha"},
			{"Not a number", "NaN", "+Inf", "NaN", "6", "Inf"},
		}, nil, false)

		it := plan.Initiatives[0]
		Expect(it.KitPct).To(BeZero())
		Expect(it.ProgressPct).To(BeZero())
		Expect(it.CostOfDelayPerWeek).To(BeZero())
		Expect(it.Work["Alpha"].Weeks).To(BeZero(), "the team estimate has the same hazard")

		_, err := json.Marshal(plan)
		Expect(err).NotTo(HaveOccurred(), "a plan carrying NaN cannot be sent to the browser")
	})

	It("ignores a blank or unparseable cell rather than inventing a value", func() {
		plan := ParseMatrix([][]string{
			{"Initiative", "Priority", "Target Date", "Tier", "Kit %", "Full Kit Estimate total", "Alpha"},
			{"Half filled", "", "TBD", "not a tier", "", "6", "6"},
		}, nil, false)

		it := plan.Initiatives[0]
		Expect(it.StatedPriority).To(BeZero())
		Expect(it.TargetDate).To(BeEmpty(), "TBD is not a date and must not become one")
		Expect(it.Tier).To(BeZero())
		Expect(it.KitPct).To(BeZero())
	})

	// A team column must never be mistaken for an attribute: they sit to the right
	// of the full-kit total, and a pod could easily be called something like "Tier".
	It("does not read a team column as an attribute", func() {
		plan := ParseMatrix([][]string{
			{"Initiative", "Full Kit Estimate total", "Tier", "Priority"},
			{"Confusing pod names", "9", "4", "3"},
		}, nil, false)

		it := plan.Initiatives[0]
		Expect(it.Tier).To(BeZero(), "Tier here is a pod, not the requester tier")
		Expect(it.StatedPriority).To(BeZero())
		Expect(it.Work).To(HaveKey("Tier"))
		Expect(it.Work["Tier"].Weeks).To(Equal(4.0))
		Expect(it.Work["Priority"].Weeks).To(Equal(3.0))
	})
})

// The sample workbook is how a manager discovers these columns exist: it is what
// GET /api/sample/initiatives.xlsx serves. If the writer omits them, the feature is
// real and invisible.
var _ = Describe("the sample initiatives workbook", func() {
	teams := []Team{{Name: "Alpha", Tracks: 3}, {Name: "Beta", Tracks: 2}}
	inits := []Initiative{
		{
			Name: "Dated and locked",
			Work: map[string]TeamWork{
				"Alpha": {Weeks: 5, Estimated: true, InPath: true},
				"Beta":  {Weeks: 3, Estimated: true, InPath: true, DependsOn: []string{"Alpha"}},
			},
			Leads:          map[string]string{"pm": "Ann", "eng": "Bo"},
			StatedPriority: 2, PriorityLocked: true,
			TargetDate: "2026-03-30", DateLocked: true,
			Tier: 1, CostOfDelayPerWeek: 8, EarliestStart: "2026-01-19",
			AfterInitiatives: []string{"Platform base"},
			KitPct:           0.75, InFlight: true, ProgressPct: 0.4,
		},
		{
			Name:  "Plain one",
			Work:  map[string]TeamWork{"Alpha": {Weeks: 2, Estimated: true, InPath: true}},
			Leads: map[string]string{"pm": "Cy"},
		},
	}

	It("round-trips every sequencing attribute back through the parser", func() {
		grid, err := ReadGrid(WriteInitiativesXLSX(teams, inits), "")
		Expect(err).NotTo(HaveOccurred())
		plan := ParseMatrix(grid, []string{"Alpha", "Beta"}, true)

		Expect(plan.Initiatives).To(HaveLen(2))
		got := plan.Initiatives[0]
		Expect(got.Name).To(Equal("Dated and locked"))
		Expect(got.StatedPriority).To(Equal(2))
		Expect(got.PriorityLocked).To(BeTrue())
		Expect(got.TargetDate).To(Equal("2026-03-30"))
		Expect(got.DateLocked).To(BeTrue())
		Expect(got.Tier).To(Equal(1))
		Expect(got.CostOfDelayPerWeek).To(Equal(8.0))
		Expect(got.EarliestStart).To(Equal("2026-01-19"))
		Expect(got.AfterInitiatives).To(Equal([]string{"Platform base"}))
		Expect(got.KitPct).To(Equal(0.75))
		Expect(got.InFlight).To(BeTrue())
		Expect(got.ProgressPct).To(Equal(0.4))

		By("keeping the team work and dependency columns intact")
		Expect(got.Work["Alpha"].Weeks).To(Equal(5.0))
		Expect(got.Work["Beta"].Weeks).To(Equal(3.0))
		Expect(got.Work["Beta"].DependsOn).To(Equal([]string{"Alpha"}))
	})

	It("leaves the cells blank for an initiative carrying no attributes", func() {
		grid, err := ReadGrid(WriteInitiativesXLSX(teams, inits), "")
		Expect(err).NotTo(HaveOccurred())
		plan := ParseMatrix(grid, []string{"Alpha", "Beta"}, true)

		plain := plan.Initiatives[1]
		Expect(plain.Name).To(Equal("Plain one"))
		Expect(plain.StatedPriority).To(BeZero())
		Expect(plain.PriorityLocked).To(BeFalse())
		Expect(plain.TargetDate).To(BeEmpty())
		Expect(plain.Tier).To(BeZero())
		Expect(plain.KitPct).To(BeZero())
		Expect(plain.InFlight).To(BeFalse())
	})

	// The predecessor cell is comma-separated, so a name containing a comma would
	// otherwise come back as two predecessors that match nothing — and an unmatched
	// predecessor is ignored, so the precedence would be lost in silence.
	It("round-trips a predecessor whose name contains a comma", func() {
		commaNamed := []Initiative{
			{Name: "Payments, phase 2", Work: map[string]TeamWork{"Alpha": {Weeks: 4, Estimated: true, InPath: true}}},
			{
				Name:             "Depends on it",
				Work:             map[string]TeamWork{"Alpha": {Weeks: 2, Estimated: true, InPath: true}},
				AfterInitiatives: []string{"Payments, phase 2", "Card rails"},
			},
		}
		grid, err := ReadGrid(WriteInitiativesXLSX(teams, commaNamed), "")
		Expect(err).NotTo(HaveOccurred())
		plan := ParseMatrix(grid, []string{"Alpha", "Beta"}, true)

		Expect(plan.Initiatives).To(HaveLen(2))
		Expect(plan.Initiatives[0].Name).To(Equal("Payments, phase 2"))
		Expect(plan.Initiatives[1].AfterInitiatives).To(Equal([]string{"Payments, phase 2", "Card rails"}))
	})

	// Quoting is what holds a comma'd name together, so the quote character itself
	// has to survive being quoted — otherwise the fix for commas renames anything
	// containing a quote, and drops that precedence just as silently.
	It("round-trips a predecessor whose name contains a double quote", func() {
		quoteNamed := []Initiative{
			{Name: `Project "Phoenix", phase 2`, Work: map[string]TeamWork{"Alpha": {Weeks: 4, Estimated: true, InPath: true}}},
			{
				Name:             "Downstream",
				Work:             map[string]TeamWork{"Alpha": {Weeks: 2, Estimated: true, InPath: true}},
				AfterInitiatives: []string{`Project "Phoenix", phase 2`},
			},
		}
		grid, err := ReadGrid(WriteInitiativesXLSX(teams, quoteNamed), "")
		Expect(err).NotTo(HaveOccurred())
		plan := ParseMatrix(grid, []string{"Alpha", "Beta"}, true)

		Expect(plan.Initiatives).To(HaveLen(2))
		Expect(plan.Initiatives[0].Name).To(Equal(`Project "Phoenix", phase 2`))
		Expect(plan.Initiatives[1].AfterInitiatives).To(Equal([]string{`Project "Phoenix", phase 2`}),
			"the predecessor must still name the same initiative after a round trip")
	})

	It("names each column so a planner can see what to fill in", func() {
		grid, err := ReadGrid(WriteInitiativesXLSX(teams, inits), "")
		Expect(err).NotTo(HaveOccurred())
		header := strings.ToLower(strings.Join(grid[0], " | "))
		for _, want := range []string{
			"priority", "priority fixed", "target date", "date fixed", "tier",
			"cost of delay", "earliest start", "depends on initiative",
			"kit %", "in flight", "% complete",
		} {
			Expect(header).To(ContainSubstring(want))
		}
	})
})
