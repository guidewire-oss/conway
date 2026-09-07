package planning

import (
	"fmt"
	"strings"
	"time"
)

type ReviewSnapshot struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	CreatedAt int64  `json:"capturedAt"`
}
type ReviewBaseline struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CreatedAt   int64  `json:"createdAt"`
	PeriodStart string `json:"periodStart,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}
type ReviewPrevious struct {
	ID              string          `json:"id"`
	ReviewDate      string          `json:"reviewDate"`
	Snapshot        *ReviewSnapshot `json:"snapshot"`
	Baseline        *ReviewBaseline `json:"baseline"`
	PlanFingerprint string          `json:"planFingerprint"`
}
type ReviewFilters struct {
	Team       string `json:"team"`
	Initiative string `json:"initiative"`
}
type ReviewAction struct {
	ID         string `json:"id"`
	PlanID     string `json:"planId"`
	Action     string `json:"action"`
	Owner      string `json:"owner"`
	ReviewDate string `json:"reviewDate"`
	Rationale  string `json:"rationale"`
	Initiative string `json:"initiative"`
	SnapshotID string `json:"snapshotId"`
	BaselineID string `json:"baselineId"`
	CreatedBy  string `json:"createdBy"`
	CreatedAt  int64  `json:"createdAt"`
	Status     string `json:"status"`
	Version    int    `json:"version"`
	Overdue    bool   `json:"overdue"`
}
type ReviewContext struct {
	ReviewDate      string          `json:"reviewDate"`
	Timezone        string          `json:"timezone"`
	PlanID          string          `json:"planId"`
	PlanFingerprint string          `json:"planFingerprint"`
	Snapshot        *ReviewSnapshot `json:"snapshot"`
	Baseline        *ReviewBaseline `json:"baseline"`
	PreviousReview  *ReviewPrevious `json:"previousReview"`
	ComparisonLabel string          `json:"comparisonLabel"`
}
type ReviewEntry struct {
	Kind       string `json:"kind"`
	Initiative string `json:"initiative,omitempty"`
	Team       string `json:"team,omitempty"`
	Reason     string `json:"reason"`
}
type ReviewAgenda struct {
	Delivery []ReviewEntry `json:"delivery"`
	Gaps     []ReviewEntry `json:"gaps"`
}
type ReviewCounts struct {
	Delivery        int `json:"delivery"`
	Gaps            int `json:"gaps"`
	OpenActions     int `json:"openActions"`
	OverdueActions  int `json:"overdueActions"`
	ResolvedActions int `json:"resolvedActions"`
}
type ReviewSummary struct {
	Context  ReviewContext     `json:"context"`
	Agenda   ReviewAgenda      `json:"agenda"`
	Actions  []ReviewAction    `json:"actions"`
	Counts   ReviewCounts      `json:"counts"`
	Filters  ReviewFilters     `json:"filters"`
	Evidence *ExecutionActuals `json:"evidence"`
}
type ReviewInput struct {
	ReviewDate, Timezone, PlanID, PlanFingerprint string
	Snapshot                                      *ReviewSnapshot
	Baseline                                      *ReviewBaseline
	Actuals                                       *ExecutionActuals
	Actions                                       []ReviewAction
	Previous                                      *ReviewPrevious
	PreviousEvidence                              *ExecutionActuals
	Filters                                       ReviewFilters
	Initiatives                                   []Initiative
}

func ValidateReviewDateZone(date, zone string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("review date must be YYYY-MM-DD")
	}
	if zone == "" || zone == "Local" {
		return fmt.Errorf("choose an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return fmt.Errorf("choose a valid IANA timezone")
	}
	return nil
}

func ValidateActionTransition(current, next, evidence string) error {
	switch next {
	case "open", "in_progress", "resolved", "superseded":
	default:
		return fmt.Errorf("status must be open, in_progress, resolved or superseded")
	}
	if current == next {
		return fmt.Errorf("the action already has this status")
	}
	if len(evidence) > 10000 {
		return fmt.Errorf("transition evidence must be within 10000 characters")
	}
	if (next == "resolved" || next == "superseded") && strings.TrimSpace(evidence) == "" {
		return fmt.Errorf("resolution or supersession requires evidence")
	}
	return nil
}

// BuildWeeklyReview summarizes existing measurements, never inferring new
// progress between captures. specs/024-weekly-execution-review.md:265
func BuildWeeklyReview(in ReviewInput) (ReviewSummary, error) {
	if err := ValidateReviewDateZone(in.ReviewDate, in.Timezone); err != nil {
		return ReviewSummary{}, err
	}
	out := ReviewSummary{Context: ReviewContext{ReviewDate: in.ReviewDate, Timezone: in.Timezone, PlanID: in.PlanID, PlanFingerprint: in.PlanFingerprint, Snapshot: in.Snapshot, Baseline: in.Baseline, PreviousReview: in.Previous}, Filters: in.Filters, Agenda: ReviewAgenda{Delivery: []ReviewEntry{}, Gaps: []ReviewEntry{}}, Actions: append([]ReviewAction{}, in.Actions...)}
	seen := map[ReviewEntry]bool{}
	add := func(entry ReviewEntry, gap bool) {
		if seen[entry] {
			return
		}
		seen[entry] = true
		if gap {
			out.Agenda.Gaps = append(out.Agenda.Gaps, entry)
		} else {
			out.Agenda.Delivery = append(out.Agenda.Delivery, entry)
		}
	}
	comparable := false
	switch {
	case in.Snapshot == nil:
		out.Context.ComparisonLabel = "Manual review: no snapshot selected; measured progress and movement are unavailable."
		add(ReviewEntry{Kind: "no-snapshot", Reason: out.Context.ComparisonLabel}, true)
	case in.Previous == nil:
		out.Context.ComparisonLabel = "First completed-review comparison: no previous review is available."
	case in.Previous.Snapshot == nil:
		out.Context.ComparisonLabel = "The preceding review was manual; no snapshot-to-snapshot movement can be measured."
	case in.Baseline == nil || in.Previous.Baseline == nil || in.Baseline.ID != in.Previous.Baseline.ID || in.Baseline.Fingerprint != in.Previous.Baseline.Fingerprint:
		out.Context.ComparisonLabel = "Agreement context changed or is missing; movement is not directly comparable."
	case in.PlanFingerprint != in.Previous.PlanFingerprint:
		out.Context.ComparisonLabel = "Working plan or scope changed; movement is not directly comparable."
	case in.Snapshot.ID == in.Previous.Snapshot.ID:
		out.Context.ComparisonLabel = "No new capture: this review uses the same snapshot as the preceding review."
	default:
		out.Context.ComparisonLabel = "New capture with unchanged agreement and working-plan context; compare only available matching measurements."
		comparable = true
	}
	if in.Snapshot != nil && in.Previous != nil && in.Previous.Snapshot != nil && in.Snapshot.ID == in.Previous.Snapshot.ID && !strings.Contains(out.Context.ComparisonLabel, "No new capture") {
		out.Context.ComparisonLabel = "No new capture: the snapshot is unchanged. " + out.Context.ComparisonLabel
	}
	// Different IDs alone cannot establish forward movement. specs/024-weekly-execution-review.md:366
	if in.Snapshot != nil && in.Previous != nil && in.Previous.Snapshot != nil && in.Snapshot.ID != in.Previous.Snapshot.ID && in.Snapshot.CreatedAt > 0 && in.Previous.Snapshot.CreatedAt > 0 && in.Snapshot.CreatedAt <= in.Previous.Snapshot.CreatedAt {
		label := "The selected capture is not newer than the preceding review's capture; directional movement is unavailable."
		if comparable {
			out.Context.ComparisonLabel = label
		} else {
			out.Context.ComparisonLabel = label + " " + out.Context.ComparisonLabel
		}
		comparable = false
	}
	if in.Snapshot != nil {
		switch in.Snapshot.Source {
		case "baseline", "template":
			add(ReviewEntry{Kind: "synthetic-evidence", Reason: "This snapshot is a baseline or template; it is synthetic evidence, not a live Jira delivery observation."}, true)
		case "jira":
		default:
			add(ReviewEntry{Kind: "unknown-provenance", Reason: "The snapshot source is unknown; its provenance cannot be treated as a Jira delivery observation."}, true)
		}
		if in.Actuals == nil {
			add(ReviewEntry{Kind: "missing-evidence", Reason: "Snapshot measurements are unavailable."}, true)
		} else {
			out.Evidence = in.Actuals
			for _, gap := range in.Actuals.Gaps {
				add(ReviewEntry{Kind: "evidence-gap", Reason: gap}, true)
			}
			for _, it := range in.Actuals.Initiatives {
				// Prefer causal team evidence per dimension. specs/024-weekly-execution-review.md:271
				sliceStart, sliceFinish := false, false
				for _, sl := range it.Slices {
					sliceStart = sliceStart || (sl.StartVarianceWeeks != nil && *sl.StartVarianceWeeks > 0)
					sliceFinish = sliceFinish || (sl.FinishVarianceWeeks != nil && *sl.FinishVarianceWeeks > 0)
				}
				if it.Status == "late" || it.Status == "at-risk" {
					add(ReviewEntry{Kind: "delivery-risk", Initiative: it.Name, Reason: "Captured evidence reports " + it.Status + " against agreement."}, false)
				}
				aggregateStart := !sliceStart && it.StartVarianceWeeks != nil && *it.StartVarianceWeeks > 0
				aggregateFinish := !sliceFinish && it.FinishVarianceWeeks != nil && *it.FinishVarianceWeeks > 0
				if aggregateStart || aggregateFinish {
					add(ReviewEntry{Kind: "agreement-divergence", Initiative: it.Name, Reason: reviewDivergenceReason("initiative", aggregateStart, aggregateFinish)}, false)
				}
				if len(it.AddedEpics)+len(it.RemovedEpics)+len(it.UnplannedPods) > 0 {
					add(ReviewEntry{Kind: "scope-change", Initiative: it.Name, Reason: "Epic bindings or assigned teams differ from the agreed scope."}, false)
				}
				for _, gap := range it.Gaps {
					add(ReviewEntry{Kind: "evidence-gap", Initiative: it.Name, Reason: gap}, true)
				}
				for _, sl := range it.Slices {
					start := sl.StartVarianceWeeks != nil && *sl.StartVarianceWeeks > 0
					finish := sl.FinishVarianceWeeks != nil && *sl.FinishVarianceWeeks > 0
					if start || finish {
						add(ReviewEntry{Kind: "agreement-divergence", Initiative: it.Name, Team: sl.Pod, Reason: reviewDivergenceReason("team", start, finish)}, false)
					}
					if sl.Status == "late" || sl.Status == "at-risk" {
						add(ReviewEntry{Kind: "delivery-risk", Initiative: it.Name, Team: sl.Pod, Reason: "Captured team evidence reports " + sl.Status + " against agreement."}, false)
					}
					for _, gap := range sl.Gaps {
						add(ReviewEntry{Kind: "evidence-gap", Initiative: it.Name, Team: sl.Pod, Reason: gap}, true)
					}
				}
			}
		}
	}
	if in.Baseline == nil {
		add(ReviewEntry{Kind: "no-agreement", Reason: "No active agreement is available; schedule comparison is unavailable."}, true)
	}
	if comparable && in.Actuals != nil && in.PreviousEvidence != nil {
		old := map[string]InitiativeActual{}
		for _, it := range in.PreviousEvidence.Initiatives {
			old[it.Name] = it
		}
		for _, it := range in.Actuals.Initiatives {
			if prior, ok := old[it.Name]; ok && it.PercentComplete != nil && prior.PercentComplete != nil && *it.PercentComplete < *prior.PercentComplete {
				add(ReviewEntry{Kind: "observed-progress-regression", Initiative: it.Name, Reason: fmt.Sprintf("Captured child-issue completion decreased from %.1f%% to %.1f%%; a changed issue mix or status can cause this, so it does not prove effort was lost.", *prior.PercentComplete, *it.PercentComplete)}, false)
			}
		}
	}
	// Filters scope the agenda and linked actions; plan-wide actions stay visible.
	membership := map[string]bool{}
	for _, it := range in.Initiatives {
		if work, ok := it.Work[in.Filters.Team]; ok && work.InPath {
			membership[it.Name] = true
		}
	}
	if in.Actuals != nil {
		for _, it := range in.Actuals.Initiatives {
			for _, sl := range it.Slices {
				if sl.Pod == in.Filters.Team {
					membership[it.Name] = true
				}
			}
		}
	}
	include := func(initiative, team string) bool {
		if initiative == "" {
			return true
		}
		if in.Filters.Initiative != "" && initiative != in.Filters.Initiative {
			return false
		}
		return in.Filters.Team == "" || (team != "" && team == in.Filters.Team) || (team == "" && membership[initiative])
	}
	filterEntries := func(entries []ReviewEntry) []ReviewEntry {
		kept := []ReviewEntry{}
		for _, e := range entries {
			if include(e.Initiative, e.Team) {
				kept = append(kept, e)
			}
		}
		return kept
	}
	out.Agenda.Delivery = filterEntries(out.Agenda.Delivery)
	out.Agenda.Gaps = filterEntries(out.Agenda.Gaps)
	filteredActions := []ReviewAction{}
	for _, a := range out.Actions {
		if include(a.Initiative, "") {
			filteredActions = append(filteredActions, a)
		}
	}
	out.Actions = filteredActions
	for i := range out.Actions {
		a := &out.Actions[i]
		if a.Status == "" {
			a.Status = "open"
		}
		if a.Version == 0 {
			a.Version = 1
		}
		a.Overdue = false
		if a.Status == "open" || a.Status == "in_progress" {
			out.Counts.OpenActions++
			if _, err := time.Parse("2006-01-02", a.ReviewDate); err == nil {
				a.Overdue = a.ReviewDate < in.ReviewDate
			} else {
				add(ReviewEntry{Kind: "action-date-gap", Initiative: a.Initiative, Reason: "Action " + a.Action + " has no valid due review date."}, true)
			}
			if a.Overdue {
				out.Counts.OverdueActions++
			}
		}
		if a.Status == "resolved" {
			out.Counts.ResolvedActions++
		}
	}
	out.Counts.Delivery = len(out.Agenda.Delivery)
	out.Counts.Gaps = len(out.Agenda.Gaps)
	return out, nil
}

func reviewDivergenceReason(scope string, start, finish bool) string {
	dimension := "finish"
	if start {
		dimension = "start"
		if finish {
			dimension = "start and finish"
		}
	}
	return "Available " + scope + " " + dimension + " evidence is later than the agreed schedule; inferred dates retain their original limitations."
}
