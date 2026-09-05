package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("execution evidence", func() {
	var inits []Initiative
	var baseline BaselineInputs
	var schedule Schedule
	var issues []ExecutionIssue
	var origin time.Time
	date := func(s string) *time.Time {
		v, err := time.Parse("2006-01-02", s)
		Expect(err).NotTo(HaveOccurred())
		return &v
	}
	BeforeEach(func() {
		origin = *date("2026-01-05")
		inits = []Initiative{{Name: "Atlas", EpicKeys: []string{"PROJ-1"}, Work: map[string]TeamWork{"Beacon": {Weeks: 6, InPath: true, Estimated: true}}}}
		baseline = NewBaselineInputs(nil, inits, Params{}, SchedulingParams{PeriodStart: "2026-01-05"})
		schedule = Schedule{PeriodStart: "2026-01-05", Initiatives: []ScheduledInitiative{{Name: "Atlas", BufferWeeks: 2, Slices: []WorkSlice{{Pod: "Beacon", StartWeek: 4, FinishWeek: 10}}}}}
		issues = []ExecutionIssue{{Key: "PROJ-1", Type: "Epic", Summary: "Atlas"}, {Key: "PROJ-2", ParentKey: "PROJ-1", Pod: "Beacon", StatusCategory: "done", Created: date("2026-02-23"), Resolved: date("2026-04-27")}}
	})
	It("separates inferred schedule variance from completed elapsed estimate bias with snapshot evidence", func() {
		a := DeriveActuals(inits, &baseline, &schedule, issues, origin.AddDate(0, 0, 112))
		slice := a.Initiatives[0].Slices[0]
		Expect(a.Coverage.Tracked).To(Equal(1))
		Expect(*slice.ActualStartWeek).To(Equal(7.0))
		Expect(*slice.StartVarianceWeeks).To(Equal(3.0))
		Expect(*slice.EstimateVariancePct).To(Equal(50.0))
		Expect(slice.StartInferred).To(BeTrue())
		Expect(slice.Confidence).To(Equal("low"))
		Expect(a.Calibration[0].Factor).To(Equal(1.5))
		Expect(inits[0].EpicKeys).To(Equal([]string{"PROJ-1"}))
	})
	It("never turns missing dates, teams, or baseline into reassuring zero variance", func() {
		issues[1].Pod = ""
		a := DeriveActuals(inits, nil, nil, issues, origin)
		Expect(a.Initiatives[0].Gaps).To(ContainElement("PROJ-2 has no team assignment."))
		Expect(a.Initiatives[0].Slices[0].PercentComplete).To(BeNil())
		Expect(a.Initiatives[0].Slices[0].StartVarianceWeeks).To(BeNil())
		Expect(a.Adherence.Percent).To(BeNil())
		Expect(a.Calibration).To(BeEmpty())
	})
	It("suggests matching epics without binding or claiming actuals", func() {
		inits[0].EpicKeys = nil
		a := DeriveActuals(inits, &baseline, &schedule, issues, origin)
		Expect(a.Coverage.Bound).To(BeZero())
		Expect(a.Initiatives[0].Tracked).To(BeFalse())
		Expect(a.Initiatives[0].Suggestions[0].Key).To(Equal("PROJ-1"))
		Expect(inits[0].EpicKeys).To(BeNil())
		Expect(a.Initiatives[0].Slices[0].Pod).To(Equal("Beacon"))
		Expect(a.Initiatives[0].Slices[0].PercentComplete).To(BeNil())
		Expect(a.Initiatives[0].RemovedEpics).To(Equal([]string{"PROJ-1"}))
		Expect(a.Initiatives[0].OriginalScopeSlices[0].EstimateVariancePct).NotTo(BeNil())
	})
	It("walks parent epic hierarchy and keeps unrelated team work separate", func() {
		issues = append(issues, ExecutionIssue{Key: "PROJ-3", Type: "Epic", ParentKey: "PROJ-1"}, ExecutionIssue{Key: "PROJ-4", ParentKey: "PROJ-3", Pod: "Delta", StatusCategory: "new"})
		a := DeriveActuals(inits, &baseline, &schedule, issues, origin)
		Expect(a.Initiatives[0].UnplannedPods).To(Equal([]string{"Delta"}))
		Expect(a.Initiatives[0].Slices[0].DoneCount).To(Equal(1))
		Expect(a.Initiatives[0].Slices[1].DoneCount).To(BeZero())
	})
	It("withholds combined-scope attribution when bound epics change", func() {
		inits[0].EpicKeys = append(inits[0].EpicKeys, "PROJ-3")
		a := DeriveActuals(inits, &baseline, &schedule, issues, origin)
		Expect(a.Initiatives[0].AddedEpics).To(Equal([]string{"PROJ-3"}))
		Expect(a.Initiatives[0].Slices[0].EstimateVariancePct).To(BeNil())
		Expect(*a.Initiatives[0].OriginalScopeSlices[0].EstimateVariancePct).To(Equal(50.0))
		Expect(a.Calibration).To(BeEmpty())
	})
	It("marks buffer risk before nominal finish instead of treating percent complete as health", func() {
		schedule.Initiatives[0].StartWeek = 0
		schedule.Initiatives[0].RawFinishWeek = 10
		schedule.Initiatives[0].CommitWeek = 12
		schedule.Initiatives[0].BufferWeeks = 2
		issues[1].Created = date("2026-01-05")
		issues[1].Resolved = date("2026-02-09")
		issues = append(issues, ExecutionIssue{Key: "PROJ-3", ParentKey: "PROJ-1", Pod: "Beacon", StatusCategory: "indeterminate", Created: date("2026-01-05")})
		a := DeriveActuals(inits, &baseline, &schedule, issues, origin.Add(1142*time.Hour))
		Expect(*a.Initiatives[0].PercentComplete).To(Equal(50.0))
		Expect(a.Initiatives[0].Status).To(Equal("late"))
		Expect(*a.Initiatives[0].BufferUsedPct).To(BeNumerically("~", 90, 1))
		Expect(a.Initiatives[0].ActualFinishWeek).To(BeNil())
	})
	It("keeps undated periods and missing resolution timestamps unknown", func() {
		schedule.PeriodStart = ""
		issues[1].Resolved = nil
		a := DeriveActuals(inits, &baseline, &schedule, issues, origin)
		slice := a.Initiatives[0].Slices[0]
		Expect(slice.ActualStartWeek).To(BeNil())
		Expect(slice.ActualFinishWeek).To(BeNil())
		Expect(slice.StartVarianceWeeks).To(BeNil())
		Expect(slice.EstimateVariancePct).To(BeNil())
		Expect(slice.Gaps).To(ContainElement("A completed issue has no resolution timestamp; finish is unknown."))
	})
	It("forecasts remaining calendar duration conditionally from snapshot progress and agreed start", func() {
		issues[1].Created = date("2026-02-02")
		issues[1].Resolved = date("2026-02-16")
		issues = append(issues, ExecutionIssue{Key: "PROJ-3", ParentKey: "PROJ-1", Pod: "Beacon", StatusCategory: "indeterminate", Created: date("2026-02-02")})
		before := baseline.Fingerprint()
		a := DeriveActuals(inits, &baseline, &schedule, issues, origin.AddDate(0, 0, 56))
		slice := a.Initiatives[0].Slices[0]
		Expect(*slice.RemainingWeeks).To(Equal(3.0))
		Expect(*slice.ForecastFinishWeek).To(Equal(11.0))
		Expect(slice.ForecastBasis).To(ContainSubstring("Conditional baseline-rate proxy as of this snapshot"))
		Expect(slice.ActualFinishWeek).To(BeNil())
		Expect(baseline.Fingerprint()).To(Equal(before))
		later := DeriveActuals(inits, &baseline, &schedule, issues, origin.AddDate(0, 0, 63))
		Expect(*later.Initiatives[0].Slices[0].ForecastFinishWeek).To(Equal(12.0))
		earlier := DeriveActuals(inits, &baseline, &schedule, issues, origin.AddDate(0, 0, 14))
		Expect(*earlier.Initiatives[0].Slices[0].ForecastFinishWeek).To(Equal(7.0), "future agreed start is the earliest assumed start")
	})
	It("withholds forecasts for missing snapshot evidence, missing period, changed scope and unmapped teams", func() {
		Expect(DeriveActuals(inits, &baseline, &schedule, issues, time.Time{}).Initiatives[0].Slices[0].RemainingWeeks).To(BeNil())
		Expect(DeriveActuals(inits, &baseline, &schedule, nil, origin).Initiatives[0].Slices[0].ForecastFinishWeek).To(BeNil())
		undated := schedule
		undated.PeriodStart = ""
		Expect(DeriveActuals(inits, &baseline, &undated, issues, origin).Initiatives[0].Slices[0].RemainingWeeks).To(BeNil())
		changed := copyInitiatives(inits)
		changed[0].EpicKeys = append(changed[0].EpicKeys, "PROJ-9")
		slice := DeriveActuals(changed, &baseline, &schedule, issues, origin).Initiatives[0].Slices[0]
		Expect(slice.RemainingWeeks).To(BeNil())
		Expect(slice.Gaps).To(ContainElement(ContainSubstring("changed or unmapped scope")))
		unmapped := append([]ExecutionIssue{}, issues...)
		unmapped = append(unmapped, ExecutionIssue{Key: "PROJ-3", ParentKey: "PROJ-1", Pod: "Unplanned", StatusCategory: "new"})
		Expect(DeriveActuals(inits, &baseline, &schedule, unmapped, origin).Initiatives[0].Slices[0].RemainingWeeks).To(BeNil())
		issues[1].StatusCategory = ""
		Expect(DeriveActuals(inits, &baseline, &schedule, issues, origin).Initiatives[0].Slices[0].RemainingWeeks).To(BeNil())
	})
	It("reports zero remaining for complete work without inventing missing finish dates", func() {
		slice := DeriveActuals(inits, &baseline, &schedule, issues, origin.AddDate(0, 0, 119)).Initiatives[0].Slices[0]
		Expect(*slice.RemainingWeeks).To(BeZero())
		Expect(slice.ForecastFinishWeek).To(BeNil())
		Expect(*slice.ActualFinishWeek).To(Equal(16.0))
		issues[1].Resolved = nil
		slice = DeriveActuals(inits, &baseline, &schedule, issues, origin.AddDate(0, 0, 119)).Initiatives[0].Slices[0]
		Expect(*slice.RemainingWeeks).To(BeZero())
		Expect(slice.ForecastFinishWeek).To(BeNil())
		Expect(slice.ActualFinishWeek).To(BeNil())
	})
	It("retains known counts while withholding progress-dependent conclusions for unknown status categories", func() {
		for _, status := range []string{"", "unrecognized"} {
			mixed := append([]ExecutionIssue{}, issues...)
			mixed = append(mixed, ExecutionIssue{Key: "PROJ-3", ParentKey: "PROJ-1", Pod: "Beacon", StatusCategory: status, Created: date("2026-02-02")})
			a := DeriveActuals(inits, &baseline, &schedule, mixed, origin.AddDate(0, 0, 119))
			it := a.Initiatives[0]
			slice := it.Slices[0]
			Expect(it.IssueCount).To(Equal(2))
			Expect(it.DoneCount).To(Equal(1))
			Expect(it.UnknownStatusCount).To(Equal(1))
			Expect(it.PercentComplete).To(BeNil())
			Expect(it.BufferUsedPct).To(BeNil())
			Expect(it.Status).To(Equal("unknown"))
			Expect(slice.IssueCount).To(Equal(2))
			Expect(slice.DoneCount).To(Equal(1))
			Expect(slice.UnknownStatusCount).To(Equal(1))
			Expect(slice.PercentComplete).To(BeNil())
			Expect(slice.BufferUsedPct).To(BeNil())
			Expect(slice.Status).To(Equal("unknown"))
			Expect(slice.EstimateVariancePct).To(BeNil())
			Expect(slice.RemainingWeeks).To(BeNil())
			Expect(slice.ForecastFinishWeek).To(BeNil())
			Expect(a.Calibration).To(BeEmpty())
			Expect(slice.Gaps).To(ContainElement(ContainSubstring("unknown status categories")))
		}
	})
	It("reports inverted start order per team and deterministic calibration", func() {
		inits = append(inits, Initiative{Name: "Beacon", EpicKeys: []string{"PROJ-5"}})
		baseline.Initiatives = append(baseline.Initiatives, inits[1])
		schedule.Initiatives = append(schedule.Initiatives, ScheduledInitiative{Name: "Beacon", Slices: []WorkSlice{{Pod: "Beacon", StartWeek: 11, FinishWeek: 13}}})
		issues = append(issues, ExecutionIssue{Key: "PROJ-5", Type: "Epic"}, ExecutionIssue{Key: "PROJ-6", ParentKey: "PROJ-5", Pod: "Beacon", StatusCategory: "done", Created: date("2026-01-12"), Resolved: date("2026-01-26")})
		a := DeriveActuals(inits, &baseline, &schedule, issues, origin)
		Expect(a.Adherence.Compared).To(Equal(1))
		Expect(a.Adherence.Followed).To(BeZero())
		Expect(a.Adherence.OutOfOrderPods).To(Equal([]string{"Beacon"}))
		Expect(DeriveActuals(inits, &baseline, &schedule, issues, origin)).To(Equal(a))
	})
	It("validates epic edits atomically and freezes their fingerprint and drag maps", func() {
		inits[0].PinnedStarts = map[string]int{"Beacon": 4}
		inits[0].PinnedLanes = map[string]int{"Beacon": 1}
		frozen := NewBaselineInputs(nil, inits, Params{}, SchedulingParams{})
		fingerprint := frozen.Fingerprint()
		inits[0].EpicKeys[0] = "PROJ-9"
		inits[0].PinnedStarts["Beacon"] = 9
		inits[0].PinnedLanes["Beacon"] = 2
		Expect(frozen.Fingerprint()).To(Equal(fingerprint))
		bad := []string{"invalid"}
		_, err := ApplyInitiativeEdits(inits, []InitiativeEdit{{Name: "Atlas", EpicKeys: &bad}}, SchedulingParams{}, 26)
		Expect(err).To(HaveOccurred())
		good := []string{"proj-2", "PROJ-2", "PROJ-1"}
		edited, err := ApplyInitiativeEdits(inits, []InitiativeEdit{{Name: "Atlas", EpicKeys: &good}}, SchedulingParams{}, 26)
		Expect(err).NotTo(HaveOccurred())
		Expect(edited[0].EpicKeys).To(Equal([]string{"PROJ-1", "PROJ-2"}))
		Expect(inits[0].EpicKeys).To(Equal([]string{"PROJ-9"}))
	})
	It("round trips explicit epic bindings in the planning workbook", func() {
		rows, err := ReadGrid(WriteInitiativesXLSX([]Team{{Name: "Beacon", Tracks: 1}}, inits), "FullKit exercise")
		Expect(err).NotTo(HaveOccurred())
		parsed := ParseMatrix(rows, []string{"Beacon"}, false)
		Expect(parsed.Initiatives[0].EpicKeys).To(Equal([]string{"PROJ-1"}))
	})
})
