package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

type ReadyCheck struct {
	Key      string `json:"key"`
	Checked  bool   `json:"checked"`
	Owner    string `json:"owner"`
	Evidence string `json:"evidence"`
}
type ReadyChecklistItem struct {
	ReadyCheck
	Label    string `json:"label"`
	Required bool   `json:"required"`
}
type ReadyConfirmation struct {
	EventOrder      int64        `json:"eventOrder"`
	ID              string       `json:"id"`
	PlanID          string       `json:"planId"`
	Team            string       `json:"team"`
	Initiative      string       `json:"initiative"`
	AsOfWeek        int          `json:"asOfWeek"`
	PlanFingerprint string       `json:"planFingerprint"`
	Checks          []ReadyCheck `json:"checks"`
	CreatedBy       string       `json:"createdBy"`
	CreatedAt       int64        `json:"createdAt"`
}
type ReleaseDecision struct {
	EventOrder      int64  `json:"eventOrder"`
	ID              string `json:"id"`
	PlanID          string `json:"planId"`
	Team            string `json:"team"`
	Initiative      string `json:"initiative"`
	AsOfWeek        int    `json:"asOfWeek"`
	PlanFingerprint string `json:"planFingerprint"`
	Decision        string `json:"decision"`
	Owner           string `json:"owner"`
	Evidence        string `json:"evidence"`
	CreatedBy       string `json:"createdBy"`
	CreatedAt       int64  `json:"createdAt"`
}
type ReadyQueueContext struct {
	PlanID           string `json:"planId"`
	PlanFingerprint  string `json:"planFingerprint"`
	Team             string `json:"team"`
	AsOfWeek         int    `json:"asOfWeek"`
	PeriodStart      string `json:"periodStart"`
	HorizonWeeks     int    `json:"horizonWeeks"`
	AcceptedOrdering string `json:"acceptedOrdering"`
	Basis            string `json:"basis"`
}
type ReadyReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Owner   string `json:"owner"`
}
type ReadyQueueItem struct {
	Initiative           string               `json:"initiative"`
	Team                 string               `json:"team"`
	State                string               `json:"state"`
	Kind                 string               `json:"kind"`
	PlannedStartWeek     *int                 `json:"plannedStartWeek"`
	PlannedFinishWeek    *int                 `json:"plannedFinishWeek"`
	EarliestFeasibleWeek *int                 `json:"earliestFeasibleWeek"`
	Reasons              []ReadyReason        `json:"reasons"`
	Checklist            []ReadyChecklistItem `json:"checklist"`
	Confirmation         *ReadyConfirmation   `json:"confirmation"`
	LastDecision         *ReleaseDecision     `json:"lastDecision"`
	ConfirmationCurrent  bool                 `json:"confirmationCurrent"`
	ReleaseCurrent       bool                 `json:"releaseCurrent"`
	CanRelease           bool                 `json:"canRelease"`
}
type ReadyQueueCounts struct {
	Total      int `json:"total"`
	InProgress int `json:"inProgress"`
	Ready      int `json:"ready"`
	Waiting    int `json:"waiting"`
	Deferred   int `json:"deferred"`
	Complete   int `json:"complete"`
}
type ReadyQueue struct {
	Fingerprint string            `json:"fingerprint"`
	Context     ReadyQueueContext `json:"context"`
	Items       []ReadyQueueItem  `json:"items"`
	Counts      ReadyQueueCounts  `json:"counts"`
}
type ReadyQueueInput struct {
	PlanID string
	// The server includes raw site configuration and plan identity here.
	PlanFingerprint string
	Inputs          BaselineInputs
	Schedule        *Schedule
	Team            string
	AsOfWeek        int
	Readiness       []ReadyConfirmation
	Decisions       []ReleaseDecision
}

var readyCheckDefinitions = []struct{ key, label string }{
	{"scope_ready", "Scope, acceptance criteria and inputs are ready"},
	{"dependencies_accepted", "Predecessor outcomes and handoffs are accepted"},
	{"team_available", "The accountable team and required people are available this week"},
}

// ValidateReadyChecks preserves partial progress without inventing confirmation.
// specs/025-team-ready-work-queue.md:281
func ValidateReadyChecks(checks []ReadyCheck) ([]ReadyCheck, error) {
	known := map[string]bool{}
	for _, def := range readyCheckDefinitions {
		known[def.key] = true
	}
	seen := map[string]ReadyCheck{}
	for _, check := range checks {
		if !known[check.Key] {
			return nil, fmt.Errorf("unknown checklist key %q", check.Key)
		}
		if _, ok := seen[check.Key]; ok {
			return nil, fmt.Errorf("duplicate checklist key %q", check.Key)
		}
		check.Owner, check.Evidence = strings.TrimSpace(check.Owner), strings.TrimSpace(check.Evidence)
		if len(check.Owner) > 500 || len(check.Evidence) > 10000 {
			return nil, fmt.Errorf("checklist owner or evidence is too long")
		}
		if check.Checked && (check.Owner == "" || check.Evidence == "") {
			return nil, fmt.Errorf("checked items require an owner and evidence")
		}
		seen[check.Key] = check
	}
	out := []ReadyCheck{}
	for _, def := range readyCheckDefinitions {
		check := seen[def.key]
		check.Key = def.key
		out = append(out, check)
	}
	return out, nil
}

// BuildReadyQueue reads accepted placement; it never fills a local gap by moving
// another slice. specs/025-team-ready-work-queue.md:259
func BuildReadyQueue(in ReadyQueueInput) (ReadyQueue, error) {
	horizon := int(math.Ceil(in.Inputs.Params.HorizonWeeks))
	if in.Schedule != nil {
		horizon = in.Schedule.HorizonWeeks
	}
	if in.AsOfWeek < 0 || in.AsOfWeek >= horizon {
		return ReadyQueue{}, fmt.Errorf("as-of week must be within the plan horizon")
	}
	teamExists := false
	for _, team := range in.Inputs.Teams {
		if team.Name == in.Team {
			teamExists = true
		}
	}
	if !teamExists || in.Team == "" {
		return ReadyQueue{}, fmt.Errorf("choose an existing plan team")
	}
	fingerprint := in.Inputs.Fingerprint()
	if in.PlanFingerprint != "" {
		fingerprint = in.PlanFingerprint
	}
	out := ReadyQueue{Context: ReadyQueueContext{PlanID: in.PlanID, PlanFingerprint: fingerprint, Team: in.Team, AsOfWeek: in.AsOfWeek, PeriodStart: in.Inputs.Scheduling.PeriodStart, HorizonWeeks: horizon, AcceptedOrdering: in.Inputs.Scheduling.AcceptedOrdering, Basis: "Current saved schedule and operational confirmations; planning weeks and release decisions are not observed starts."}, Items: []ReadyQueueItem{}}
	if out.Context.AcceptedOrdering == "" {
		out.Context.AcceptedOrdering = "engine"
	}
	scheduled := map[string]ScheduledInitiative{}
	var teamSchedule *PodSchedule
	if in.Schedule != nil {
		for _, it := range in.Schedule.Initiatives {
			scheduled[it.Name] = it
		}
		for i := range in.Schedule.PodWeeks {
			if in.Schedule.PodWeeks[i].Pod == in.Team {
				teamSchedule = &in.Schedule.PodWeeks[i]
			}
		}
	}
	for _, it := range in.Inputs.Initiatives {
		work, assigned := it.Work[in.Team]
		if !assigned || !work.InPath {
			continue
		}
		row := ReadyQueueItem{Initiative: it.Name, Team: in.Team, State: "waiting", Kind: "work", Reasons: []ReadyReason{}, Checklist: []ReadyChecklistItem{}}
		if work.Estimated && work.Weeks == 0 {
			row.Kind = "milestone"
		}
		eligible := true
		block := func(code, message, owner string) {
			eligible = false
			row.Reasons = append(row.Reasons, ReadyReason{code, message, owner})
		}
		for i := range in.Readiness {
			c := &in.Readiness[i]
			if c.Team == in.Team && c.Initiative == it.Name && c.PlanID == in.PlanID {
				row.Confirmation = c
			}
		}
		for i := range in.Decisions {
			d := &in.Decisions[i]
			if d.Team == in.Team && d.Initiative == it.Name && d.PlanID == in.PlanID {
				row.LastDecision = d
			}
		}
		checks := map[string]ReadyCheck{}
		if row.Confirmation != nil {
			row.ConfirmationCurrent = row.Confirmation.PlanFingerprint == fingerprint && row.Confirmation.AsOfWeek == in.AsOfWeek
			for _, check := range row.Confirmation.Checks {
				checks[check.Key] = check
			}
			if !row.ConfirmationCurrent {
				block("stale-confirmation", "Operational evidence belongs to different planning inputs or a different week; reconfirm the current context.", "")
			}
		}
		for _, def := range readyCheckDefinitions {
			check := checks[def.key]
			check.Key = def.key
			row.Checklist = append(row.Checklist, ReadyChecklistItem{check, def.label, true})
			if !row.ConfirmationCurrent || !check.Checked || strings.TrimSpace(check.Owner) == "" || strings.TrimSpace(check.Evidence) == "" {
				owner := check.Owner
				if owner == "" {
					owner = "Unassigned"
				}
				block("checklist", def.label+" requires current owner and evidence confirmation.", owner)
			}
		}
		si, placed := scheduled[it.Name]
		var slice *WorkSlice
		if placed {
			for i := range si.Slices {
				if si.Slices[i].Pod == in.Team {
					slice = &si.Slices[i]
					break
				}
			}
		}
		if !placed || si.Verdict == verdictUnschedulable || slice == nil || slice.StartWeek < 0 || slice.StartWeek >= horizon {
			reason := "No usable team placement is available; inspect the schedule and replan."
			if si.BindingConstraint != "" {
				reason += " Scheduler constraint: " + si.BindingConstraint + "."
			}
			block("unavailable-placement", reason, "")
		} else {
			start, finish := slice.StartWeek, slice.FinishWeek
			row.PlannedStartWeek, row.PlannedFinishWeek, row.EarliestFeasibleWeek = &start, &finish, &start
			if start > in.AsOfWeek {
				block("future-start", fmt.Sprintf("Waiting for the existing scheduled start in week %d; no earlier local slot is authorized.", start), "")
			}
			if start < in.AsOfWeek {
				block("past-start", "The planned start has passed; verify actual status or replan. A planned start is not an observed start.", "")
			}
			if !work.Estimated || (!slice.Estimated && work.Weeks > 0) {
				block("missing-estimate", "This team's estimate is missing; operational release requires known work.", "")
			}
			if si.Provisional && !readyOnlyZeroProvisional(it.Name, in.Inputs, scheduled) {
				block("provisional-chain", "The whole initiative rests on provisional or unresolved scheduling inputs.", "")
			}
			for _, sl := range si.Slices {
				source := it.Work[sl.Pod]
				if !source.Estimated || sl.StartWeek >= horizon || sl.FinishWeek < sl.StartWeek {
					block("held-chain", "An upstream or downstream slice lacks usable placement or estimate; inspect the whole initiative.", "")
					break
				}
			}
			for pod, w := range it.Work {
				if w.InPath && !w.Estimated {
					block("missing-chain-estimate", "An assigned team has no estimate; the whole chain is not ready for release.", "")
					break
				}
				if w.InPath {
					found := false
					for _, sl := range si.Slices {
						if sl.Pod == pod {
							found = true
							break
						}
					}
					if !found {
						block("missing-chain-placement", "An assigned team has no scheduled slice; inspect the whole initiative.", "")
						break
					}
				}
			}
			if work.Estimated && work.Weeks == 0 {
				row.Kind = "milestone"
				row.Reasons = append(row.Reasons, ReadyReason{"acceptance-checkpoint", "Acceptance checkpoint: no work week or lane consumption is implied.", ""})
			} else if !readyReservationContinuous(teamSchedule, *slice) {
				block("interrupted-capacity", "The accepted reservation is missing, insufficient or interrupted; inspect the calendar and replan before release.", "")
			}
		}
		if in.Schedule == nil || (in.Schedule.Fit != nil && in.Schedule.Fit.UnavailableReason != "") {
			block("unavailable-schedule", "The schedule is unavailable and cannot authorize release.", "")
		}
		if in.Inputs.Scheduling.KitGate > 0 && it.KitPct < in.Inputs.Scheduling.KitGate {
			block("kit-gate", "The configured numeric full-kit gate still holds; operational checks do not change KitPct.", "")
		}
		if len(si.Assumptions) > 0 {
			for _, assumption := range si.Assumptions {
				row.Reasons = append(row.Reasons, ReadyReason{"schedule-assumption", assumption, ""})
			}
		}
		if row.LastDecision != nil {
			d := row.LastDecision
			row.ReleaseCurrent = d.Decision == "release" && d.PlanFingerprint == fingerprint && d.AsOfWeek == in.AsOfWeek
			if row.ReleaseCurrent {
				block("released", "Release recorded; start remains unconfirmed.", d.Owner)
			} else if d.Decision == "release" {
				row.Reasons = append(row.Reasons, ReadyReason{"historical-release", "An earlier release belongs to different planning inputs or a different week and does not authorize this context.", d.Owner})
			}
		}
		if eligible {
			row.State = "ready"
			row.CanRelease = true
		}
		if row.LastDecision != nil && row.LastDecision.Decision == "defer" {
			row.State = "deferred"
			row.CanRelease = false
			row.Reasons = append(row.Reasons, ReadyReason{"deferred", "Deliberately deferred until explicit reconsideration: " + row.LastDecision.Evidence, row.LastDecision.Owner})
		}
		if it.InFlight {
			row.State = "in_progress"
			row.CanRelease = false
			row.Reasons = append(row.Reasons, ReadyReason{"reported-carryover", "Reported initiative carryover; this does not confirm the selected team's activity.", ""})
			if it.ProgressPct >= 1 {
				row.State = "complete"
				row.Reasons = append(row.Reasons, ReadyReason{"declared-complete", "Declared complete initiative context; no new release opportunity is implied.", ""})
			}
		}
		out.Items = append(out.Items, row)
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, aok := scheduled[out.Items[i].Initiative]
		b, bok := scheduled[out.Items[j].Initiative]
		return aok && (!bok || a.ProposedRank < b.ProposedRank)
	})
	for _, row := range out.Items {
		out.Counts.Total++
		switch row.State {
		case "in_progress":
			out.Counts.InProgress++
		case "ready":
			out.Counts.Ready++
		case "waiting":
			out.Counts.Waiting++
		case "deferred":
			out.Counts.Deferred++
		case "complete":
			out.Counts.Complete++
		}
	}
	raw, err := json.Marshal(struct {
		PlanID          string
		PlanFingerprint string
		Inputs          BaselineInputs
		Team            string
		Week            int
		Readiness       []ReadyConfirmation
		Decisions       []ReleaseDecision
	}{in.PlanID, fingerprint, in.Inputs, in.Team, in.AsOfWeek, in.Readiness, in.Decisions})
	if err != nil {
		return ReadyQueue{}, err
	}
	sum := sha256.Sum256(raw)
	out.Fingerprint = hex.EncodeToString(sum[:])
	return out, nil
}

func readyReservationContinuous(pod *PodSchedule, sl WorkSlice) bool {
	if pod == nil {
		return false
	}
	last := -1
	for _, week := range pod.Weeks {
		occupied := false
		for _, name := range week.Initiatives {
			if name == sl.Initiative {
				occupied = true
				break
			}
		}
		if !occupied {
			continue
		}
		width := sl.LanesUsed
		if width < 1 {
			width = 1
		}
		if len(sl.Phases) > 0 {
			width = 0
			for _, phase := range sl.Phases {
				if week.Week >= phase.FromWeek && week.Week < phase.ToWeek {
					width = phase.Lanes
					break
				}
			}
		}
		if width < 1 || week.Tracks < width || week.Busy < width || week.Busy > week.Tracks || (last < 0 && week.Week != sl.StartWeek) || (last >= 0 && week.Week != last+1) {
			return false
		}
		last = week.Week
	}
	return last >= sl.StartWeek && last-sl.StartWeek+1 >= int(math.Ceil(sl.RemainingWeeks))
}

// Explicit zero effort is an acceptance checkpoint, while unresolved graphs
// and genuinely missing estimates still block it. specs/025-team-ready-work-queue.md:316
func readyOnlyZeroProvisional(name string, inputs BaselineInputs, scheduled map[string]ScheduledInitiative) bool {
	byName := map[string]Initiative{}
	teams := map[string]bool{}
	for _, it := range inputs.Initiatives {
		byName[it.Name] = it
	}
	for _, team := range inputs.Teams {
		teams[team.Name] = true
	}
	visiting := map[string]bool{}
	var inspect func(string) (bool, bool)
	inspect = func(name string) (bool, bool) {
		it, exists := byName[name]
		si, placed := scheduled[name]
		if !exists || !placed || visiting[name] || si.Verdict == verdictUnschedulable {
			return false, false
		}
		visiting[name] = true
		defer delete(visiting, name)
		inPath := map[string]bool{}
		zero := false
		for pod, work := range it.Work {
			if !work.InPath {
				continue
			}
			if !teams[pod] || !work.Estimated || work.Weeks < 0 {
				return false, false
			}
			inPath[pod] = true
		}
		_, _, broken := podOrder(it.Work, inPath)
		if len(broken) > 0 {
			return false, false
		}
		for _, pod := range si.UnestimatedPods {
			work, present := it.Work[pod]
			if !present || !work.InPath || !work.Estimated || work.Weeks != 0 {
				return false, false
			}
			zero = true
		}
		for _, predecessor := range it.AfterInitiatives {
			valid, predecessorZero := inspect(predecessor)
			if !valid {
				return false, false
			}
			zero = zero || predecessorZero
		}
		return true, zero
	}
	valid, zero := inspect(name)
	return valid && zero
}
