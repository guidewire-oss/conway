package main

import (
	"conway/server/planning"
	"encoding/json"
	"fmt"
	"time"
)

func backend() {
	h := []string{"Initiative", "Full Kit Estimate total", "Atlas Sequence", "Atlas"}
	for _, est := range []string{"TBD", ""} {
		p := planning.ParseMatrix([][]string{h, {"First", "", "NONE", est}}, []string{"Atlas"}, false)
		fmt.Printf("estimate %q work=%+v\n", est, p.Initiatives[0].Work)
	}
	dups := planning.ParseMatrix([][]string{{"Initiative", "Full Kit Estimate total", "Atlas", "Atlas"}, {"First", "", "3", "7"}}, []string{"Atlas"}, false)
	fmt.Printf("duplicate team columns teams=%v work=%+v\n", dups.Teams, dups.Initiatives[0].Work)
	ts, err := planning.ParseTeamsRows([][]string{{"Team", "Tracks"}, {"Atlas", "2"}, {"Atlas", "5"}})
	fmt.Printf("duplicate roster rows=%+v err=%v\n", ts, err)
	z := 0.0
	sp := planning.SchedulingParams{PeriodStart: "2026-09-01", EstimateModel: "effort", WipModel: "off", BufferPct: &z}
	params := planning.Params{HorizonWeeks: 26}
	a := planning.Initiative{Name: "First", Work: map[string]planning.TeamWork{"Atlas": {Weeks: 4, Estimated: true, InPath: true}}}
	before := planning.ComputeSchedule([]planning.Team{{Name: "Atlas", Tracks: 1}}, []planning.Initiative{a}, params, sp)
	sp.PeriodStart = "2026-10-27"
	after := planning.ComputeSchedule([]planning.Team{{Name: "Atlas", Tracks: 1}}, []planning.Initiative{a}, params, sp)
	fmt.Printf("eight-week period shift comparison=%+v\n", planning.CompareToBaseline(before, after))
	sp.KitGate = 1
	held := planning.ComputeSchedule([]planning.Team{{Name: "Atlas", Tracks: 1}}, []planning.Initiative{a}, params, sp)
	fmt.Printf("held comparison=%+v\n", planning.CompareToBaseline(after, held))
	sp.KitGate = 0
	a.Work["Beacon"] = planning.TeamWork{InPath: true, DependsOn: []string{"Atlas"}}
	teams := []planning.Team{{Name: "Atlas", Tracks: 1}, {Name: "Beacon", Tracks: 1}}
	b := planning.ComputeSchedule(teams, []planning.Initiative{a}, params, sp)
	t2, i2 := planning.ApplyLevers(teams, []planning.Initiative{a}, []planning.Lever{{Type: "reassign", Pod: "Beacon", ToPod: "Atlas"}})
	now := planning.ComputeSchedule(t2, i2, params, sp)
	fmt.Printf("reassign unknown beforeProvisional=%t afterProvisional=%t work=%+v\n", b.Initiatives[0].Provisional, now.Initiatives[0].Provisional, i2[0].Work)
}

func imports() {
	zero := 0.0
	sp := planning.SchedulingParams{PeriodStart: "2026-09-01", WipModel: "off", EstimateModel: "wall-clock", BufferPct: &zero}
	params := planning.Params{HorizonWeeks: 26}
	teams := []planning.Team{{Name: "Atlas", Tracks: 1}, {Name: "Beacon", Tracks: 1}}
	header := []string{"Initiative", "Full Kit Estimate total", "Atlas Sequence", "Atlas", "Beacon Sequence", "Beacon"}
	for _, dep := range []string{"Atlas", "ATLAS"} {
		p := planning.ParseMatrix([][]string{header, {"First", "", "NONE", "4", dep, "1"}}, []string{"Atlas", "Beacon"}, true)
		s := planning.ComputeScheduleWith(teams, p.Initiatives, params, sp, planning.ScheduleOptions{})
		fmt.Printf("dependency%q slices=%+v provisional=%t\n", dep, s.Initiatives[0].Slices, s.Initiatives[0].Provisional)
	}
	p := planning.ParseMatrix([][]string{header, {"First", "", "NONE", "4", "NONE", "TBD"}}, []string{"Atlas", "Beacon"}, false)
	s := planning.ComputeScheduleWith(teams, p.Initiatives, params, sp, planning.ScheduleOptions{})
	fmt.Printf("TBD known plus unknown work=%v provisional=%t unestimated=%v\n", p.Initiatives[0].Work, s.Initiatives[0].Provisional, s.Initiatives[0].UnestimatedPods)
	dups, _ := planning.ParseTeamsRows([][]string{{"Team", "Tracks"}, {"Atlas", "2"}, {"Atlas", "5"}})
	s = planning.ComputeScheduleWith(dups, []planning.Initiative{{Name: "First", Work: map[string]planning.TeamWork{"Atlas": {Weeks: 4, Estimated: true, InPath: true}}}}, params, sp, planning.ScheduleOptions{})
	fmt.Printf("duplicate roster scheduleTracks=%d fitAvailable=%g expectedForUsedTracks=%d\n", s.PodWeeks[0].Tracks, s.Fit.TrackWeeksAvailable, s.PodWeeks[0].Tracks*26)
}

func actuals() {
	date := func(s string) *time.Time {
		v, e := time.Parse("2006-01-02", s)
		if e != nil {
			panic(e)
		}
		return &v
	}
	inits := []planning.Initiative{{Name: "First", EpicKeys: []string{"PROJ-1", "PROJ-9"}, Work: map[string]planning.TeamWork{"Atlas": {Weeks: 4, Estimated: true, InPath: true}}}}
	b := planning.NewBaselineInputs([]planning.Team{{Name: "Atlas", Tracks: 1}}, inits, planning.Params{HorizonWeeks: 26}, planning.SchedulingParams{PeriodStart: "2026-09-01"})
	s := planning.Schedule{PeriodStart: "2026-09-01", Initiatives: []planning.ScheduledInitiative{{Name: "First", StartWeek: 0, RawFinishWeek: 4, CommitWeek: 5, BufferWeeks: 1, Slices: []planning.WorkSlice{{Pod: "Atlas", StartWeek: 0, FinishWeek: 4, RemainingWeeks: 4, LanesUsed: 1, Estimated: true}}}}}
	issues := []planning.ExecutionIssue{{Key: "PROJ-1", Type: "Epic", Summary: "First"}, {Key: "PROJ-2", Type: "Task", ParentKey: "PROJ-1", Pod: "Atlas", StatusCategory: "done", Created: date("2026-09-08"), Resolved: date("2026-09-22")}}
	a := planning.DeriveActuals(inits, &b, &s, issues, *date("2026-10-06"))
	raw, _ := json.MarshalIndent(a, "", "  ")
	fmt.Println(string(raw))
}

func main() { backend(); imports(); actuals() }
