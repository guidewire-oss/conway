package planning

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

var ErrValidationLimit = errors.New("history validation limit exceeded")

type ValidationCounts struct {
	Eligible        int      `json:"eligible"`
	Covered         int      `json:"covered"`
	Before          int      `json:"before"`
	After           int      `json:"after"`
	Pending         int      `json:"pending"`
	Excluded        int      `json:"excluded"`
	Repeated        int      `json:"repeated"`
	CoveragePercent *float64 `json:"coveragePercent"`
}
type ValidationMonth struct {
	Month string `json:"month"`
	ValidationCounts
}
type ValidationRow struct {
	PredictionOutcome
	PredictionID             string `json:"predictionId"`
	PredictionName           string `json:"predictionName"`
	IssuedAt                 int64  `json:"issuedAt"`
	Month                    string `json:"month"`
	RepresentativeID         string `json:"representativeId"`
	RepresentativeInitiative string `json:"representativeInitiative"`
}
type PredictionValidation struct {
	ValidationCounts
	ReferenceID       string             `json:"referenceId"`
	Settings          ForecastSettings   `json:"settings"`
	Evidence          PredictionEvidence `json:"evidence"`
	TotalRecords      int                `json:"totalRecords"`
	CandidateRecords  int                `json:"candidateRecords"`
	MismatchedRecords int                `json:"mismatchedRecords"`
	TooLateRecords    int                `json:"tooLateRecords"`
	Months            []ValidationMonth  `json:"months"`
	Rows              []ValidationRow    `json:"rows"`
}

func ValidationMatches(reference, candidate ForecastPrediction) bool {
	return reference.Evidence.SourceID != "" && reference.Evidence.ConfigFingerprint != "" && reference.Evidence.SourceID == candidate.Evidence.SourceID && reference.Evidence.ConfigFingerprint == candidate.Evidence.ConfigFingerprint && reference.Forecast.Settings == candidate.Forecast.Settings
}

func (v *ValidationCounts) count(status string) {
	switch status {
	case "within":
		v.Eligible++
		v.Covered++
	case "before":
		v.Eligible++
		v.Before++
	case "after":
		v.Eligible++
		v.After++
	case "pending":
		v.Pending++
	case "repeated":
		v.Repeated++
	default:
		v.Excluded++
	}
	if v.Eligible > 0 {
		pct := 100 * float64(v.Covered) / float64(v.Eligible)
		v.CoveragePercent = &pct
	}
}

// specs/029-forecast-history-validation.md:123: choose connected-scope representatives
// before examining outcomes, including transitive overlap and excluded originals.
func ValidatePredictionHistory(reference ForecastPrediction, history []ForecastPrediction, current []Initiative, evidence PredictionEvidence, issues []ExecutionIssue) (PredictionValidation, error) {
	out := PredictionValidation{ReferenceID: reference.ID, Settings: reference.Forecast.Settings, Evidence: evidence, TotalRecords: len(history), Months: []ValidationMonth{}, Rows: []ValidationRow{}}
	if len(history) > 200 {
		return out, fmt.Errorf("%w: this report supports at most 200 recorded predictions. No partial report was produced; retain history and contact an administrator", ErrValidationLimit)
	}
	if evidence.SourceID == "" || evidence.SourceID != reference.Evidence.SourceID || evidence.ConfigFingerprint == "" || evidence.ConfigFingerprint != reference.Evidence.ConfigFingerprint {
		return out, errors.New("choose a capture with the reference prediction's source and extraction settings")
	}
	candidates := []ForecastPrediction{}
	entries := 0
	for _, p := range history {
		if !ValidationMatches(reference, p) {
			out.MismatchedRecords++
			continue
		}
		if p.IssuedAt >= evidence.StartedAt {
			out.TooLateRecords++
			continue
		}
		entries += len(p.Inputs.Initiatives)
		if entries > 5000 {
			return out, fmt.Errorf("%w: this report supports at most 5000 candidate initiative entries. No partial report was produced; retain history and contact an administrator", ErrValidationLimit)
		}
		candidates = append(candidates, p)
	}
	out.CandidateRecords = len(candidates)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].IssuedAt != candidates[j].IssuedAt {
			return candidates[i].IssuedAt < candidates[j].IssuedAt
		}
		return candidates[i].ID < candidates[j].ID
	})
	type entry struct {
		record     int
		initiative int
	}
	scope := []entry{}
	parents := []int{}
	keys := map[string]int{}
	root := func(i int) int {
		for parents[i] != i {
			parents[i] = parents[parents[i]]
			i = parents[i]
		}
		return i
	}
	for r, p := range candidates {
		order := make([]int, len(p.Inputs.Initiatives))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool { return p.Inputs.Initiatives[order[i]].Name < p.Inputs.Initiatives[order[j]].Name })
		for _, i := range order {
			index := len(scope)
			scope = append(scope, entry{r, i})
			parents = append(parents, index)
			it := p.Inputs.Initiatives[i]
			allKeys := append([]string{}, it.EpicKeys...)
			for _, v := range predictionScope(it, p.Issues) {
				allKeys = append(allKeys, v.Key)
			}
			for _, key := range allKeys {
				if key == "" {
					continue
				}
				if earlier, ok := keys[key]; ok {
					a, b := root(index), root(earlier)
					if a != b {
						parents[max(a, b)] = min(a, b)
					}
				} else {
					keys[key] = index
				}
			}
		}
	}
	assessments := map[int]PredictionAssessment{}
	months := map[string]*ValidationMonth{}
	for index, e := range scope {
		p := candidates[e.record]
		it := p.Inputs.Initiatives[e.initiative]
		first := scope[root(index)]
		original := candidates[first.record]
		row := ValidationRow{PredictionID: p.ID, PredictionName: p.Name, IssuedAt: p.IssuedAt, Month: time.Unix(p.IssuedAt, 0).UTC().Format("2006-01"), RepresentativeID: original.ID, RepresentativeInitiative: original.Inputs.Initiatives[first.initiative].Name}
		if root(index) != index {
			row.PredictionOutcome = PredictionOutcome{Name: it.Name, Status: "repeated", Reason: "Shared captured work is represented by the earliest recorded prediction; later results do not replace it."}
		} else {
			assessment, ok := assessments[e.record]
			if !ok {
				assessment = AssessPrediction(p, current, evidence, issues)
				assessments[e.record] = assessment
			}
			row.PredictionOutcome = assessment.Rows[e.initiative]
			month := months[row.Month]
			if month == nil {
				month = &ValidationMonth{Month: row.Month}
				months[row.Month] = month
			}
			month.count(row.Status)
		}
		out.count(row.Status)
		out.Rows = append(out.Rows, row)
	}
	for _, month := range months {
		out.Months = append(out.Months, *month)
	}
	sort.Slice(out.Months, func(i, j int) bool { return out.Months[i].Month < out.Months[j].Month })
	return out, nil
}
