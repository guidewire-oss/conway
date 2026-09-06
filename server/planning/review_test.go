package planning

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// specs/024-weekly-execution-review.md:56: agenda observations must distinguish
// measured delivery exceptions from unavailable evidence.
var _ = Describe("weekly execution review agenda", func() {
	input := func() ReviewInput {
		return ReviewInput{PlanID: "plan-a", PlanFingerprint: "inputs-a", ReviewDate: "2026-11-01", Timezone: "America/Los_Angeles"}
	}
	jsonText := func(v any) string {
		data, err := json.Marshal(v)
		Expect(err).NotTo(HaveOccurred())
		return string(data)
	}
	It("permits a manual review without manufacturing measured progress", func() {
		summary, err := BuildWeeklyReview(input())
		Expect(err).NotTo(HaveOccurred())
		Expect(summary.Context.Snapshot).To(BeNil())
		Expect(summary.Evidence).To(BeNil())
		Expect(summary.Agenda.Delivery).To(BeEmpty())
		Expect(summary.Agenda.Gaps).NotTo(BeEmpty())
		Expect(strings.ToLower(jsonText(summary.Agenda.Gaps))).To(ContainSubstring("snapshot"))
	})
	DescribeTable("rejects invalid review date and timezone context", func(date, zone string) {
		in := input()
		in.ReviewDate, in.Timezone = date, zone
		_, err := BuildWeeklyReview(in)
		Expect(err).To(HaveOccurred())
	}, Entry("impossible date", "2026-02-30", "UTC"),
		Entry("timestamp instead of civil date", "2026-11-01T00:00:00Z", "UTC"),
		Entry("missing date", "", "UTC"),
		Entry("unknown zone", "2026-11-01", "Invalid/Place"),
		Entry("missing zone", "2026-11-01", ""))
	It("uses the chosen civil date across a DST boundary and excludes closed actions", func() {
		in := input()
		in.Actions = []ReviewAction{
			{ID: "before", ReviewDate: "2026-10-31", Status: "open", Version: 1},
			{ID: "same", ReviewDate: "2026-11-01", Status: "in_progress", Version: 2},
			{ID: "after", ReviewDate: "2026-11-02", Status: "open", Version: 1},
			{ID: "resolved", ReviewDate: "2026-10-30", Status: "resolved", Version: 3},
			{ID: "superseded", ReviewDate: "2026-10-30", Status: "superseded", Version: 2},
		}
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		overdue := []string{}
		for _, action := range summary.Actions {
			if action.Overdue {
				overdue = append(overdue, action.ID)
			}
		}
		Expect(overdue).To(ConsistOf("before"))
		Expect(summary.Counts.OverdueActions).To(Equal(1))
		Expect(summary.Context.Timezone).To(Equal("America/Los_Angeles"))
	})
	It("separates measured lateness from missing epic evidence without mutating actuals", func() {
		in := input()
		in.Snapshot = &ReviewSnapshot{ID: "snapshot-a", Source: "jira"}
		in.Actuals = &ExecutionActuals{Initiatives: []InitiativeActual{
			{Name: "Atlas", Tracked: true, Status: "late", FinishVarianceWeeks: number(2)},
			{Name: "Beacon", Status: "unknown", Gaps: []string{"Bound epic is absent from this snapshot."}},
		}}
		before := jsonText(in.Actuals)
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(jsonText(summary.Agenda.Delivery)).To(ContainSubstring("Atlas"))
		Expect(jsonText(summary.Agenda.Delivery)).NotTo(ContainSubstring("Beacon"))
		Expect(jsonText(summary.Agenda.Gaps)).To(ContainSubstring("Beacon"))
		Expect(jsonText(in.Actuals)).To(Equal(before))
		Expect(summary.Evidence.Initiatives[1].PercentComplete).To(BeNil())
		Expect(summary.Evidence.Initiatives[1].ActualFinishWeek).To(BeNil())
	})
	It("labels the same captured evidence instead of inventing progress since last review", func() {
		in := input()
		in.Snapshot = &ReviewSnapshot{ID: "snapshot-a", Source: "jira"}
		in.Baseline = &ReviewBaseline{ID: "agreement-a"}
		in.Previous = &ReviewPrevious{ID: "review-a", ReviewDate: "2026-10-25", Snapshot: in.Snapshot, Baseline: in.Baseline, PlanFingerprint: in.PlanFingerprint}
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.ToLower(summary.Context.ComparisonLabel)).To(ContainSubstring("no new capture"))
		Expect(summary.Context.PreviousReview.ID).To(Equal("review-a"))
	})
	It("qualifies a changed agreement rather than treating it as directly comparable delivery", func() {
		in := input()
		in.Snapshot = &ReviewSnapshot{ID: "snapshot-b", Source: "jira"}
		in.Baseline = &ReviewBaseline{ID: "agreement-b"}
		in.Previous = &ReviewPrevious{ID: "review-a", ReviewDate: "2026-10-25", Snapshot: &ReviewSnapshot{ID: "snapshot-a"}, Baseline: &ReviewBaseline{ID: "agreement-a"}, PlanFingerprint: in.PlanFingerprint}
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.ToLower(summary.Context.ComparisonLabel)).To(ContainSubstring("agreement"))
		Expect(summary.Context.Baseline.ID).To(Equal("agreement-b"))
		Expect(summary.Context.PreviousReview.Baseline.ID).To(Equal("agreement-a"))
	})
	// specs/024-weekly-execution-review.md:62: reused evidence and a changed
	// agreement are independent facts; both must remain visible.
	It("discloses a reused capture even when the agreement also changed", func() {
		in := input()
		in.Snapshot = &ReviewSnapshot{ID: "snapshot-a", Source: "jira"}
		in.Baseline = &ReviewBaseline{ID: "agreement-b"}
		in.Previous = &ReviewPrevious{ID: "review-a", Snapshot: in.Snapshot, Baseline: &ReviewBaseline{ID: "agreement-a"}, PlanFingerprint: in.PlanFingerprint}
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		label := strings.ToLower(summary.Context.ComparisonLabel)
		Expect(label).To(ContainSubstring("no new capture"))
		Expect(label).To(ContainSubstring("agreement"))
	})
	// specs/024-weekly-execution-review.md:141: missing provenance cannot be
	// positively classified as a known synthetic or measured source.
	It("distinguishes unknown snapshot provenance from a known synthetic example", func() {
		in := input()
		in.Snapshot = &ReviewSnapshot{ID: "snapshot-a"}
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		gaps := strings.ToLower(jsonText(summary.Agenda.Gaps))
		Expect(gaps).To(ContainSubstring("unknown"))
		Expect(gaps).NotTo(ContainSubstring("it is synthetic"))
	})
	// specs/024-weekly-execution-review.md:348: filtering uses source membership
	// so a team retains assigned initiatives that have no measured slices.
	It("filters by team input membership while preserving untracked work and plan-wide actions", func() {
		in := input()
		in.Filters = ReviewFilters{Team: "Team A"}
		in.Initiatives = []Initiative{
			{Name: "Atlas", Work: map[string]TeamWork{"Team A": {InPath: true}}},
			{Name: "Beacon", Work: map[string]TeamWork{"Team B": {InPath: true}}},
		}
		in.Snapshot = &ReviewSnapshot{ID: "snapshot-a", Source: "jira"}
		in.Actuals = &ExecutionActuals{Initiatives: []InitiativeActual{
			{Name: "Atlas", Status: "not-tracked", Gaps: []string{"No bound epic for this assigned initiative."}},
			{Name: "Beacon", Status: "late", Gaps: []string{"Team B gap."}},
		}}
		in.Actions = []ReviewAction{{ID: "plan", Status: "open", ReviewDate: "2026-11-01"}, {ID: "a", Initiative: "Atlas", Status: "open", ReviewDate: "2026-10-30"}, {ID: "b", Initiative: "Beacon", Status: "open", ReviewDate: "2026-10-30"}}
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(jsonText(summary.Agenda.Gaps)).To(ContainSubstring("Atlas"))
		Expect(jsonText(summary.Agenda)).NotTo(ContainSubstring("Beacon"))
		Expect(summary.Actions).To(HaveLen(2))
		Expect(summary.Counts.OpenActions).To(Equal(2))
		Expect(summary.Counts.OverdueActions).To(Equal(1))
		Expect(summary.Evidence.Initiatives).To(HaveLen(2), "frozen evidence is not truncated by presentation filters")
	})
	// specs/024-weekly-execution-review.md:353: positive observation movement
	// does not create a delivery exception; decreased issue-count completion does.
	It("compares available completion values without calling an increase a delivery exception", func() {
		in := input()
		in.Snapshot = &ReviewSnapshot{ID: "snapshot-b", Source: "jira"}
		in.Baseline = &ReviewBaseline{ID: "agreement-a"}
		in.Previous = &ReviewPrevious{ID: "review-a", Snapshot: &ReviewSnapshot{ID: "snapshot-a", Source: "jira"}, Baseline: in.Baseline, PlanFingerprint: in.PlanFingerprint}
		in.PreviousEvidence = &ExecutionActuals{Initiatives: []InitiativeActual{{Name: "Atlas", PercentComplete: number(50)}}}
		in.Actuals = &ExecutionActuals{Initiatives: []InitiativeActual{{Name: "Atlas", PercentComplete: number(75)}}}
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(summary.Agenda.Delivery).To(BeEmpty())
		in.Actuals.Initiatives[0].PercentComplete = number(25)
		summary, err = BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(jsonText(summary.Agenda.Delivery)).To(ContainSubstring("Atlas"))
		Expect(strings.ToLower(jsonText(summary.Agenda.Delivery))).To(ContainSubstring("issue"))
	})
	// specs/024-weekly-execution-review.md:359: a different ID does not make an
	// older or simultaneous observation a later delivery measurement.
	DescribeTable("does not infer directional regression from a snapshot that is not newer", func(capturedAt int64) {
		in := input()
		in.Snapshot = &ReviewSnapshot{ID: "selected-capture", Source: "jira", CreatedAt: capturedAt}
		in.Baseline = &ReviewBaseline{ID: "agreement-a"}
		in.Previous = &ReviewPrevious{ID: "review-a", Snapshot: &ReviewSnapshot{ID: "prior-capture", Source: "jira", CreatedAt: 1793491200}, Baseline: in.Baseline, PlanFingerprint: in.PlanFingerprint}
		in.PreviousEvidence = &ExecutionActuals{Initiatives: []InitiativeActual{{Name: "Atlas", PercentComplete: number(75)}}}
		in.Actuals = &ExecutionActuals{Initiatives: []InitiativeActual{{Name: "Atlas", PercentComplete: number(25)}}}
		summary, err := BuildWeeklyReview(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.ToLower(summary.Context.ComparisonLabel)).To(ContainSubstring("not newer"))
		Expect(summary.Agenda.Delivery).To(BeEmpty())
	}, Entry("older capture", int64(1793404800)), Entry("equal capture time", int64(1793491200)))
})
