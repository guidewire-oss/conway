package main

import (
	"conway/server/planning"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strings"
)

// specs/027-evidence-linked-planning-assistant.md:206
var _ = Describe("assistant comparison boundaries", func() {
	placed := func() planning.ScheduledInitiative {
		return planning.ScheduledInitiative{Name: "Beacon", StartWeek: 3, CommitWeek: 5, Verdict: "fits", Slices: []planning.WorkSlice{{Pod: "Atlas", StartWeek: 3, FinishWeek: 4}}}
	}
	input := planning.BaselineInputs{Initiatives: []planning.Initiative{{Name: "Beacon", Work: map[string]planning.TeamWork{"Atlas": {Weeks: 1, InPath: true}}}}}
	It("withholds dates whenever either placement is unknown", func() {
		for _, mask := range []int{1, 2, 3} {
			old, current := placed(), placed()
			if mask&1 != 0 {
				old.Slices = nil
				old.StartWeek = 0
				old.CommitWeek = 0
			}
			if mask&2 != 0 {
				current.Slices = nil
				current.StartWeek = 0
				current.CommitWeek = 0
			}
			facts := assistantChangeFacts(&planning.Schedule{PeriodStart: "2026-09-07", Initiatives: []planning.ScheduledInitiative{old}}, &planning.Schedule{PeriodStart: "2026-09-07", Initiatives: []planning.ScheduledInitiative{current}}, input, input, assistantRequest{}, "plan")
			Expect(facts).To(HaveLen(1))
			Expect(strings.Join(facts[0].Details, " ")).To(ContainSubstring("Date movement unknown"))
			Expect(strings.Join(facts[0].Details, " ")).NotTo(ContainSubstring("movement: -"))
		}
	})
	It("does not call equal relative weeks unchanged calendar dates", func() {
		for _, origin := range []string{"", "2026-09-14"} {
			facts := assistantChangeFacts(&planning.Schedule{PeriodStart: "2026-09-07", Initiatives: []planning.ScheduledInitiative{placed()}}, &planning.Schedule{PeriodStart: origin, Initiatives: []planning.ScheduledInitiative{placed()}}, input, input, assistantRequest{}, "plan")
			Expect(strings.Join(facts[0].Details, " ")).To(ContainSubstring("Date movement withheld"))
		}
	})
	It("keeps model unknowns and scoped team facts", func() {
		i := placed()
		i.Provisional = true
		i.UnestimatedPods = []string{"Atlas"}
		i.Assumptions = []string{"Missing estimate"}
		i.Slices = nil
		facts := assistantScheduleFacts(input, &planning.Schedule{Initiatives: []planning.ScheduledInitiative{i}}, assistantRequest{Team: "Atlas"}, "plan")
		Expect(facts).To(HaveLen(1))
		Expect(strings.Join(facts[0].Details, " ")).To(ContainSubstring("start and finish remain unknown"))
		Expect(strings.Join(facts[0].Details, " ")).To(ContainSubstring("Missing estimate"))
		Expect(facts[0].Kind).To(Equal("Modeled"))
	})
})
