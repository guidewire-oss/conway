package planning

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Review regression evidence boundaries", func() {
	// per specs/017-planning-and-execution-usability.md:193
	DescribeTable("retains observed counts but withholds whole-scope claims when a bound epic is absent", func(status string, secondType string) {
		origin := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		resolved := origin.AddDate(0, 0, 28)
		inits := []Initiative{{Name: "Atlas", EpicKeys: []string{"PROJ-1", "PROJ-9"}, Work: map[string]TeamWork{"Beacon": {Weeks: 6, InPath: true, Estimated: true}}}}
		baseline := NewBaselineInputs(nil, inits, Params{}, SchedulingParams{PeriodStart: "2026-01-05"})
		schedule := Schedule{PeriodStart: "2026-01-05", Initiatives: []ScheduledInitiative{{Name: "Atlas", StartWeek: 0, RawFinishWeek: 6, CommitWeek: 8, BufferWeeks: 2, Slices: []WorkSlice{{Pod: "Beacon", StartWeek: 0, FinishWeek: 6, Estimated: true}}}}}
		issues := []ExecutionIssue{{Key: "PROJ-1", Type: "Epic"}, {Key: "PROJ-2", ParentKey: "PROJ-1", Pod: "Beacon", StatusCategory: status, Created: &origin, Resolved: &resolved}}
		if secondType != "" {
			issues = append(issues, ExecutionIssue{Key: "PROJ-9", Type: secondType})
		}
		before := baseline.Fingerprint()
		actuals := DeriveActuals(inits, &baseline, &schedule, issues, origin.AddDate(0, 0, 49))
		Expect(actuals.Initiatives).To(HaveLen(1))
		initiative := actuals.Initiatives[0]
		Expect(initiative.Tracked).To(BeTrue())
		Expect(initiative.IssueCount).To(Equal(1))
		done := 0
		if status == "done" {
			done = 1
		}
		Expect(initiative.DoneCount).To(Equal(done))
		Expect(initiative.Gaps).To(ContainElement(ContainSubstring("PROJ-9")))
		Expect(initiative.PercentComplete).To(BeNil())
		Expect(initiative.ActualFinishWeek).To(BeNil())
		Expect(initiative.StartVarianceWeeks).To(BeNil())
		Expect(initiative.FinishVarianceWeeks).To(BeNil())
		Expect(initiative.BufferUsedPct).To(BeNil())
		Expect(initiative.Status).To(Equal("unknown"))
		Expect(initiative.Slices).To(HaveLen(1))
		slice := initiative.Slices[0]
		Expect(slice.IssueCount).To(Equal(1))
		Expect(slice.DoneCount).To(Equal(done))
		Expect(slice.PercentComplete).To(BeNil())
		Expect(slice.ActualFinishWeek).To(BeNil())
		Expect(slice.StartVarianceWeeks).To(BeNil())
		Expect(slice.FinishVarianceWeeks).To(BeNil())
		Expect(slice.EstimateVariancePct).To(BeNil())
		Expect(slice.BufferUsedPct).To(BeNil())
		Expect(slice.RemainingWeeks).To(BeNil())
		Expect(slice.ForecastFinishWeek).To(BeNil())
		Expect(slice.Status).To(Equal("unknown"))
		Expect(actuals.Calibration).To(BeEmpty())
		Expect(baseline.Fingerprint()).To(Equal(before))
	}, Entry("completed captured children", "done", ""), Entry("unfinished captured children", "indeterminate", ""), Entry("bound key is not an epic", "done", "Story"))

	// per specs/019-scheduling-audit-and-gantt-integrity.md:135
	DescribeTable("rejects a single empty normalized initiative identity", func(name string) {
		Expect(ValidateInitiativeNames([]Initiative{{Name: name}})).To(HaveOccurred())
	}, Entry("empty", ""), Entry("spaces", "   "), Entry("mixed whitespace", "\t\n \u00a0"))

	// per specs/001-plan-execution-order.md:732
	// per specs/019-scheduling-audit-and-gantt-integrity.md:135
	// per specs/017-planning-and-execution-usability.md:210
	It("returns an explicit unavailable fit while keeping duplicate input rows unschedulable", func() {
		teams := []Team{{Name: "Beacon", Tracks: 2}}
		inits := []Initiative{
			{Name: "Atlas", Work: map[string]TeamWork{"Beacon": {Weeks: 3, InPath: true, Estimated: true}}},
			{Name: " atlas ", Work: map[string]TeamWork{"Beacon": {Weeks: 7, InPath: true, Estimated: true}}},
		}
		schedule := ComputeScheduleWith(teams, inits, Params{HorizonWeeks: 10}, SchedulingParams{}, ScheduleOptions{})
		Expect(schedule.Fit).NotTo(BeNil())
		encoded, err := json.Marshal(schedule)
		Expect(err).NotTo(HaveOccurred())
		var payload map[string]any
		Expect(json.Unmarshal(encoded, &payload)).To(Succeed())
		fit, ok := payload["fit"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(fit).To(HaveKeyWithValue("unavailableReason", ContainSubstring("duplicate")))
		Expect(fit).To(HaveKeyWithValue("podWeeksDemanded", BeNil()))
		Expect(fit).To(HaveKeyWithValue("trackWeeksAvailable", BeNil()))
		Expect(fit).To(HaveKeyWithValue("beyondHorizon", BeNil()))
		Expect(schedule.Initiatives).To(HaveLen(2))
		Expect(schedule.UnscheduledWeight).To(BeNumerically(">", 0))
		for _, row := range schedule.Initiatives {
			Expect(row.Verdict).To(Equal(verdictUnschedulable))
			Expect(row.Provisional).To(BeTrue())
			Expect(row.BindingConstraint).To(Equal("invalid-input"))
			Expect(row.Slices).To(BeEmpty())
		}
	})
})
