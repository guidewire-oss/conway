package planning

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// specs/025-team-ready-work-queue.md:55: recommendations retain the accepted
// schedule and distinguish operational authorization from observed activity.
var _ = Describe("team ready-work queue", func() {
	input := func() ReadyQueueInput {
		in := ReadyQueueInput{PlanID: "plan-a", Team: "Team A", AsOfWeek: 0, Inputs: BaselineInputs{
			Teams:       []Team{{Name: "Team A", Tracks: 1}},
			Initiatives: []Initiative{{Name: "Atlas", StatedPriority: 1, Work: map[string]TeamWork{"Team A": {Weeks: 2, Estimated: true, InPath: true}}}, {Name: "Beacon", StatedPriority: 2, Work: map[string]TeamWork{"Team A": {Weeks: 1, Estimated: true, InPath: true}}}},
			Params:      Params{HorizonWeeks: 8}, Scheduling: SchedulingParams{PeriodStart: "2026-09-07", WipModel: WipOff, AcceptedOrdering: "stated", BufferPct: number(0)},
		}}
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		return in
	}
	confirm := func(in *ReadyQueueInput, name string) {
		in.Readiness = append(in.Readiness, ReadyConfirmation{ID: "confirmation-" + name, PlanID: in.PlanID, Team: in.Team, Initiative: name, AsOfWeek: in.AsOfWeek, PlanFingerprint: in.Inputs.Fingerprint(), Checks: []ReadyCheck{
			{Key: "scope_ready", Checked: true, Owner: "Delivery lead", Evidence: "Scope accepted for the checkpoint."},
			{Key: "dependencies_accepted", Checked: true, Owner: "Dependency owner", Evidence: "Predecessor acceptance confirmed."},
			{Key: "team_available", Checked: true, Owner: "Team lead", Evidence: "The team confirmed this week's availability."},
		}})
	}
	item := func(queue ReadyQueue, name string) ReadyQueueItem {
		for _, row := range queue.Items {
			if row.Initiative == name {
				return row
			}
		}
		Fail("assigned initiative missing from queue: " + name)
		return ReadyQueueItem{}
	}
	reasons := func(row ReadyQueueItem) string {
		b, err := json.Marshal(row.Reasons)
		Expect(err).NotTo(HaveOccurred())
		return strings.ToLower(string(b))
	}
	// specs/025-team-ready-work-queue.md:264: the omitted ordering has the
	// same stated-priority meaning in the queue and accepted scheduler.
	It("labels an omitted accepted ordering as stated priority", func() {
		in := input()
		in.Inputs.Scheduling.AcceptedOrdering = ""
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(queue.Context.AcceptedOrdering).To(Equal("stated"))
	})
	It("requires operational evidence before the current scheduled start is ready", func() {
		in := input()
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(item(queue, "Atlas").State).To(Equal("waiting"))
		Expect(item(queue, "Atlas").CanRelease).To(BeFalse())
		confirm(&in, "Atlas")
		queue, err = BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		ready := item(queue, "Atlas")
		Expect(ready.State).To(Equal("ready"))
		Expect(ready.CanRelease).To(BeTrue())
		Expect(ready.ConfirmationCurrent).To(BeTrue())
		Expect(*ready.PlannedStartWeek).To(Equal(0))
		Expect(*item(queue, "Beacon").PlannedStartWeek).To(Equal(2))
		Expect(item(queue, "Beacon").State).To(Equal("waiting"))
	})
	It("never turns an earlier forecast into an observed start", func() {
		in := input()
		in.AsOfWeek = 1
		confirm(&in, "Atlas")
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		row := item(queue, "Atlas")
		Expect(row.State).To(Equal("waiting"))
		Expect(row.CanRelease).To(BeFalse())
		Expect(reasons(row)).To(SatisfyAny(ContainSubstring("status"), ContainSubstring("replan"), ContainSubstring("past")))
	})
	It("labels initiative carryover without claiming confirmed team activity", func() {
		in := input()
		in.Inputs.Initiatives[0].InFlight = true
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		row := item(queue, "Atlas")
		Expect(row.State).To(Equal("in_progress"))
		Expect(row.CanRelease).To(BeFalse())
		Expect(reasons(row)).To(ContainSubstring("initiative"))
		Expect(reasons(row)).To(ContainSubstring("carryover"))
	})
	It("retains declared complete work without recommending it for a new release", func() {
		in := input()
		in.Inputs.Initiatives[0].InFlight = true
		in.Inputs.Initiatives[0].ProgressPct = 1
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(item(queue, "Atlas").State).To(Equal("complete"))
		Expect(item(queue, "Atlas").CanRelease).To(BeFalse())
	})
	It("keeps checklist confirmation separate from the existing numeric kit gate", func() {
		in := input()
		in.Inputs.Scheduling.KitGate = 0.8
		in.Inputs.Initiatives[0].KitPct = 0.5
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		confirm(&in, "Atlas")
		before, err := json.Marshal(in.Inputs)
		Expect(err).NotTo(HaveOccurred())
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		row := item(queue, "Atlas")
		Expect(row.State).To(Equal("waiting"))
		Expect(row.CanRelease).To(BeFalse())
		Expect(reasons(row)).To(ContainSubstring("kit"))
		after, err := json.Marshal(in.Inputs)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})
	It("retains assigned unestimated work and refuses provisional release", func() {
		in := input()
		work := in.Inputs.Initiatives[0].Work["Team A"]
		work.Estimated = false
		work.Weeks = 0
		in.Inputs.Initiatives[0].Work["Team A"] = work
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		confirm(&in, "Atlas")
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		row := item(queue, "Atlas")
		Expect(row.State).To(Equal("waiting"))
		Expect(row.CanRelease).To(BeFalse())
		Expect(reasons(row)).To(ContainSubstring("estimat"))
	})
	It("requires reconfirmation after either planning inputs or the operational week changes", func() {
		in := input()
		confirm(&in, "Atlas")
		in.Inputs.Teams[0].Tracks = 2
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(item(queue, "Atlas").ConfirmationCurrent).To(BeFalse())
		Expect(item(queue, "Atlas").CanRelease).To(BeFalse())
		in = input()
		confirm(&in, "Atlas")
		in.AsOfWeek = 1
		queue, err = BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(item(queue, "Atlas").ConfirmationCurrent).To(BeFalse())
	})
	It("records release authorization without turning it into started work", func() {
		in := input()
		confirm(&in, "Atlas")
		in.Decisions = []ReleaseDecision{{ID: "release-a", PlanID: in.PlanID, Team: in.Team, Initiative: "Atlas", AsOfWeek: 0, PlanFingerprint: in.Inputs.Fingerprint(), Decision: "release", Owner: "Team lead", Evidence: "Authorize this planned release."}}
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		row := item(queue, "Atlas")
		Expect(row.State).To(Equal("waiting"))
		Expect(row.CanRelease).To(BeFalse())
		Expect(row.ReleaseCurrent).To(BeTrue())
		Expect(reasons(row)).To(ContainSubstring("unconfirmed"))
	})
	It("keeps a deliberate deferral after input edits and requires reconsideration", func() {
		in := input()
		in.Decisions = []ReleaseDecision{{ID: "defer-a", PlanID: in.PlanID, Team: in.Team, Initiative: "Atlas", AsOfWeek: 0, PlanFingerprint: in.Inputs.Fingerprint(), Decision: "defer", Owner: "Team lead", Evidence: "Wait for stakeholder confirmation."}}
		in.Inputs.Teams[0].Tracks = 2
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(item(queue, "Atlas").State).To(Equal("deferred"))
		Expect(item(queue, "Atlas").CanRelease).To(BeFalse())
	})
	It("treats a zero-effort milestone as acceptance without inventing a work week", func() {
		in := input()
		in.Inputs.Initiatives = []Initiative{{Name: "Atlas", Work: map[string]TeamWork{"Team A": {Weeks: 0, Estimated: true, InPath: true}}}}
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		confirm(&in, "Atlas")
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		row := item(queue, "Atlas")
		Expect(row.Kind).To(Equal("milestone"))
		Expect(row.CanRelease).To(BeTrue(), "%+v", row)
		Expect(row.PlannedStartWeek).NotTo(BeNil())
		Expect(row.PlannedFinishWeek).NotTo(BeNil())
		Expect(*row.PlannedStartWeek).To(Equal(*row.PlannedFinishWeek))
		for _, pod := range in.Schedule.PodWeeks {
			for _, week := range pod.Weeks {
				Expect(week.Busy).To(Equal(0))
			}
		}
	})
	// specs/025-team-ready-work-queue.md:333: explicit-zero exceptions cannot
	// bypass a self-dependency that the accepted scheduler marks unresolved.
	It("keeps a zero-effort self-dependent checkpoint provisional and unreleasable", func() {
		in := input()
		in.Inputs.Initiatives = []Initiative{{Name: "Atlas", Work: map[string]TeamWork{"Team A": {Weeks: 0, Estimated: true, InPath: true, DependsOn: []string{"Team A"}}}}}
		in.Schedule = in.Inputs.RecomputeWith(ScheduleOptions{})
		Expect(in.Schedule.Initiatives).To(HaveLen(1))
		Expect(in.Schedule.Initiatives[0].Provisional).To(BeTrue())
		Expect(strings.ToLower(strings.Join(in.Schedule.Initiatives[0].Assumptions, " "))).To(ContainSubstring("dependency"))
		confirm(&in, "Atlas")
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(item(queue, "Atlas").CanRelease).To(BeFalse())
		Expect(item(queue, "Atlas").State).To(Equal("waiting"))
	})
	// specs/025-team-ready-work-queue.md:267: phase growth may use contiguous
	// occupied weeks; lane-weeks of effort are not the same as elapsed weeks.
	It("accepts continuous phase growth whose lane-weeks cover the planned effort", func() {
		in := input()
		in.Inputs.Teams[0].Tracks = 2
		in.Inputs.Initiatives = []Initiative{{Name: "Atlas", Work: map[string]TeamWork{"Team A": {Weeks: 5, Estimated: true, InPath: true}}}}
		in.Inputs.Scheduling.EstimateModel = EstimateEffort
		slice := WorkSlice{Initiative: "Atlas", Pod: "Team A", StartWeek: 0, FinishWeek: 3, RemainingWeeks: 3, LanesUsed: 1, Estimated: true, Phases: []LanePhase{{FromWeek: 0, ToWeek: 1, Lanes: 1}, {FromWeek: 1, ToWeek: 3, Lanes: 2}}}
		in.Schedule = &Schedule{HorizonWeeks: 8, Initiatives: []ScheduledInitiative{{Name: "Atlas", StartWeek: 0, RawFinishWeek: 3, CommitWeek: 3, Slices: []WorkSlice{slice}}}, PodWeeks: []PodSchedule{{Pod: "Team A", Tracks: 2, Slices: []WorkSlice{slice}, Weeks: []PodWeek{{Week: 0, Busy: 1, Tracks: 2, Initiatives: []string{"Atlas"}}, {Week: 1, Busy: 2, Tracks: 2, Initiatives: []string{"Atlas"}}, {Week: 2, Busy: 2, Tracks: 2, Initiatives: []string{"Atlas"}}}}}}
		confirm(&in, "Atlas")
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(item(queue, "Atlas").CanRelease).To(BeTrue(), "%+v", item(queue, "Atlas"))
	})
	// specs/025-team-ready-work-queue.md:267: a nominal span can contain a
	// calendar gap; missing idle-week initiative keys must not fabricate work.
	It("refuses a flat slice with an idle gap inside its planned reservation", func() {
		in := input()
		in.Inputs.Initiatives = in.Inputs.Initiatives[:1]
		slice := WorkSlice{Initiative: "Atlas", Pod: "Team A", StartWeek: 0, FinishWeek: 4, RemainingWeeks: 2, LanesUsed: 1, Estimated: true}
		in.Schedule = &Schedule{HorizonWeeks: 8, Initiatives: []ScheduledInitiative{{Name: "Atlas", StartWeek: 0, RawFinishWeek: 4, CommitWeek: 4, Slices: []WorkSlice{slice}}}, PodWeeks: []PodSchedule{{Pod: "Team A", Tracks: 1, Slices: []WorkSlice{slice}, Weeks: []PodWeek{{Week: 0, Busy: 1, Tracks: 1, Initiatives: []string{"Atlas"}}, {Week: 1, Busy: 0, Tracks: 1}, {Week: 2, Busy: 0, Tracks: 1}, {Week: 3, Busy: 1, Tracks: 1, Initiatives: []string{"Atlas"}}}}}}
		confirm(&in, "Atlas")
		queue, err := BuildReadyQueue(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(item(queue, "Atlas").State).To(Equal("waiting"))
		Expect(item(queue, "Atlas").CanRelease).To(BeFalse())
	})
	DescribeTable("rejects an invalid team or operational week", func(team string, week int) {
		in := input()
		in.Team, in.AsOfWeek = team, week
		_, err := BuildReadyQueue(in)
		Expect(err).To(HaveOccurred())
	}, Entry("missing team", "", 0), Entry("unknown team", "Team Z", 0), Entry("negative week", "Team A", -1), Entry("outside horizon", "Team A", 8))
})
