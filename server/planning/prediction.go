package planning

import (
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"time"
)

type PredictionEvidence struct {
	SnapshotID        string `json:"snapshotId"`
	SourceID          string `json:"sourceId"`
	ConfigFingerprint string `json:"configFingerprint"`
	StartedAt         int64  `json:"startedAt"`
	CapturedAt        int64  `json:"capturedAt"`
}
type ForecastPrediction struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	IssuedAt  int64              `json:"issuedAt"`
	CreatedBy string             `json:"createdBy"`
	Inputs    BaselineInputs     `json:"inputs"`
	Forecast  PortfolioForecast  `json:"forecast"`
	Evidence  PredictionEvidence `json:"evidence"`
	Issues    []ExecutionIssue   `json:"issues"`
}
type PredictionOutcome struct {
	Name              string   `json:"name"`
	Status            string   `json:"status"`
	Reason            string   `json:"reason"`
	EarliestFinish    string   `json:"earliestFinish"`
	LatestFinish      string   `json:"latestFinish"`
	CurrentCommitment string   `json:"currentCommitment"`
	ActualFinish      string   `json:"actualFinish"`
	VarianceWeeks     *float64 `json:"varianceWeeks"`
}
type PredictionAssessment struct {
	Rows            []PredictionOutcome `json:"rows"`
	Eligible        int                 `json:"eligible"`
	Covered         int                 `json:"covered"`
	Pending         int                 `json:"pending"`
	Excluded        int                 `json:"excluded"`
	CoveragePercent *float64            `json:"coveragePercent"`
	Evidence        PredictionEvidence  `json:"evidence"`
}

// specs/028-portfolio-forecasts.md:216: match captured membership, not issue counts.
func predictionScope(it Initiative, issues []ExecutionIssue) []ExecutionIssue {
	byKey := map[string]ExecutionIssue{}
	bound := map[string]bool{}
	for _, v := range issues {
		byKey[v.Key] = v
	}
	for _, k := range it.EpicKeys {
		bound[k] = true
	}
	out := []ExecutionIssue{}
	for _, v := range issues {
		belongs := bound[v.Key]
		parent := v.ParentKey
		seen := map[string]bool{}
		for parent != "" && !seen[parent] && !belongs {
			seen[parent] = true
			belongs = bound[parent]
			parent = byKey[parent].ParentKey
		}
		if belongs {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
func samePredictionScope(a, b []ExecutionIssue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].ParentKey != b[i].ParentKey || a[i].Pod != b[i].Pod || a[i].Type != b[i].Type {
			return false
		}
	}
	return true
}

// PredictionIssues retains only bound evidence and omits issue descriptions and summaries.
func PredictionIssues(in BaselineInputs, issues []ExecutionIssue) []ExecutionIssue {
	found := map[string]ExecutionIssue{}
	for _, it := range in.Initiatives {
		for _, v := range predictionScope(it, issues) {
			v.Summary = ""
			found[v.Key] = v
		}
	}
	out := []ExecutionIssue{}
	for _, v := range found {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// specs/028-portfolio-forecasts.md:225: coverage describes this prediction's
// completed cohort; it neither pools repeated predictions nor estimates probability.
func AssessPrediction(p ForecastPrediction, current []Initiative, evidence PredictionEvidence, issues []ExecutionIssue) PredictionAssessment {
	out := PredictionAssessment{Rows: []PredictionOutcome{}, Evidence: evidence}
	byName := map[string]Initiative{}
	for _, it := range current {
		byName[it.Name] = it
	}
	memberships := map[string]int{}
	for _, it := range p.Inputs.Initiatives {
		for _, v := range predictionScope(it, p.Issues) {
			if !epicIssue(v) {
				memberships[v.Key]++
			}
		}
	}
	for _, it := range p.Inputs.Initiatives {
		row := assessPredictionInitiative(p, it, byName[it.Name], evidence, issues, memberships)
		switch row.Status {
		case "pending":
			out.Pending++
		case "excluded":
			out.Excluded++
		default:
			out.Eligible++
			if row.Status == "within" {
				out.Covered++
			}
		}
		out.Rows = append(out.Rows, row)
	}
	if out.Eligible > 0 {
		v := 100 * float64(out.Covered) / float64(out.Eligible)
		out.CoveragePercent = &v
	}
	return out
}
func assessPredictionInitiative(p ForecastPrediction, it, current Initiative, ev PredictionEvidence, issues []ExecutionIssue, memberships map[string]int) PredictionOutcome {
	row := PredictionOutcome{Name: it.Name, Status: "excluded"}
	exclude := func(reason string) PredictionOutcome { row.Reason = reason; return row }
	if ev.SourceID == "" || ev.SourceID != p.Evidence.SourceID || ev.ConfigFingerprint == "" || ev.ConfigFingerprint != p.Evidence.ConfigFingerprint {
		return exclude("Source or extraction configuration differs from the recorded prediction.")
	}
	if ev.StartedAt <= p.IssuedAt || ev.CapturedAt < ev.StartedAt {
		return exclude("Choose a capture started after this prediction was recorded.")
	}
	a, _ := json.Marshal(it)
	b, _ := json.Marshal(current)
	if !reflect.DeepEqual(a, b) {
		return exclude("Initiative inputs changed or the initiative was removed since prediction.")
	}
	origin, err := time.Parse(isoDate, p.Inputs.Scheduling.PeriodStart)
	if err != nil {
		return exclude("The prediction has no dated planning period.")
	}
	if len(p.Forecast.Scenarios) != 3 {
		return exclude("The recorded scenario results are incomplete.")
	}
	low, high, central, commit := math.MaxInt, 0, 0, 0
	for index, s := range p.Forecast.Scenarios {
		if s.Schedule == nil {
			return exclude("A recorded scenario schedule is missing.")
		}
		found := false
		for _, r := range s.Schedule.Initiatives {
			if r.Name == it.Name {
				if !ForecastKnown(r) {
					return exclude("A recorded scenario has unknown or provisional placement.")
				}
				found = true
				low = min(low, r.RawFinishWeek)
				high = max(high, r.RawFinishWeek)
				if index == 1 {
					central = r.RawFinishWeek
					commit = r.CommitWeek
				}
				break
			}
		}
		if !found {
			return exclude("An initiative is missing from the recorded scenarios.")
		}
	}
	date := func(week int) time.Time { return origin.AddDate(0, 0, week*7) }
	row.EarliestFinish = date(low).Format(isoDate)
	row.LatestFinish = date(high).Format(isoDate)
	row.CurrentCommitment = date(commit).Format(isoDate)
	if !date(low).After(time.Unix(p.IssuedAt, 0)) {
		return exclude("The scenario envelope starts at or before prediction issuance.")
	}
	oldScope, newScope := predictionScope(it, p.Issues), predictionScope(it, issues)
	if len(it.EpicKeys) == 0 || len(oldScope) == 0 {
		return exclude("No bound issue scope was captured when the prediction was recorded.")
	}
	if !samePredictionScope(oldScope, newScope) {
		return exclude("Issue membership, parent, type or team assignment changed since prediction.")
	}
	unfinished := false
	for _, v := range oldScope {
		if !epicIssue(v) {
			if memberships[v.Key] > 1 {
				return exclude("Issue scope overlaps another initiative in this prediction.")
			}
			if v.StatusCategory != "done" {
				unfinished = true
			}
			if !knownExecutionStatus(v.StatusCategory) {
				return exclude("Initial issue statuses were incomplete.")
			}
		}
	}
	if !unfinished {
		return exclude("Work was already complete when the prediction was recorded.")
	}
	for _, v := range newScope {
		if !epicIssue(v) {
			if !knownExecutionStatus(v.StatusCategory) {
				return exclude("Some issue statuses are unknown.")
			}
			if v.StatusCategory == "done" && (v.Resolved == nil || v.Resolved.Unix() > ev.CapturedAt) {
				return exclude("Completed work has missing or future resolution timestamps.")
			}
		}
	}
	actual := DeriveActuals([]Initiative{it}, &p.Inputs, p.Forecast.Scenarios[1].Schedule, newScope, time.Unix(ev.CapturedAt, 0)).Initiatives[0]
	if len(actual.UnplannedPods) > 0 {
		return exclude("Captured work includes teams absent from the recorded initiative schedule.")
	}
	if actual.ActualFinishWeek == nil {
		if actual.Tracked && actual.UnknownStatusCount == 0 && len(actual.UnplannedPods) == 0 && len(actual.Slices) > 0 {
			completeTeams := true
			for _, s := range actual.Slices {
				if s.IssueCount == 0 {
					completeTeams = false
				}
			}
			// Missing epics and unmapped child work must not be hidden as pending.
			keys := map[string]bool{}
			children := 0
			for _, v := range newScope {
				if epicIssue(v) {
					keys[v.Key] = true
				} else {
					children++
					if v.Pod == "" {
						completeTeams = false
					}
				}
			}
			for _, k := range it.EpicKeys {
				if !keys[k] {
					completeTeams = false
				}
			}
			if completeTeams && children == actual.IssueCount && actual.DoneCount < actual.IssueCount {
				row.Status = "pending"
				row.Reason = "Comparable work is still incomplete; excluded from completed-outcome coverage."
				return row
			}
		}
		return exclude("Captured scope is incomplete; check bound epics, planned teams and resolution timestamps.")
	}
	finish := origin.Add(time.Duration(*actual.ActualFinishWeek * 168 * float64(time.Hour)))
	if finish.Unix() <= p.IssuedAt {
		return exclude("The observed finish is at or before prediction issuance.")
	}
	row.ActualFinish = finish.UTC().Format(isoDate)
	variance := *actual.ActualFinishWeek - float64(central)
	row.VarianceWeeks = &variance
	row.Status = "within"
	if finish.Before(date(low)) {
		row.Status = "before"
	} else if finish.After(date(high)) {
		row.Status = "after"
	}
	row.Reason = "Complete captured scope; observed finish compared with the saved scenario envelope."
	return row
}
