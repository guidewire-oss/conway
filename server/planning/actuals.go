package planning

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

var epicKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[0-9]+$`)

func NormalizeEpicKeys(keys []string) ([]string, error) {
	if len(keys) > 100 {
		return nil, fmt.Errorf("at most 100 epic keys may be bound")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range keys {
		key := strings.ToUpper(strings.TrimSpace(raw))
		if !epicKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("epic key %q must look like PROJ-123", raw)
		}
		if !seen[key] {
			out = append(out, key)
			seen[key] = true
		}
	}
	sort.Strings(out)
	return out, nil
}

// ExecutionIssue contains snapshot evidence only; no I/O or live Jira access.
// specs/001-plan-execution-order.md:1393: starts remain explicitly inferred.
type ExecutionIssue struct {
	Key, ParentKey, Pod, Type, Summary, StatusCategory string
	Created, Updated, Resolved                         *time.Time
}
type EpicSuggestion struct {
	Key     string  `json:"key"`
	Summary string  `json:"summary"`
	Score   float64 `json:"score"`
}
type SliceActual struct {
	Pod                 string   `json:"pod"`
	IssueCount          int      `json:"issueCount"`
	DoneCount           int      `json:"doneCount"`
	UnknownStatusCount  int      `json:"unknownStatusCount"`
	PercentComplete     *float64 `json:"percentComplete"`
	ActualStartWeek     *float64 `json:"actualStartWeek"`
	ActualFinishWeek    *float64 `json:"actualFinishWeek"`
	StartInferred       bool     `json:"startInferred"`
	Confidence          string   `json:"confidence"`
	Gaps                []string `json:"gaps"`
	BaselineStartWeek   *float64 `json:"baselineStartWeek"`
	BaselineFinishWeek  *float64 `json:"baselineFinishWeek"`
	StartVarianceWeeks  *float64 `json:"startVarianceWeeks"`
	FinishVarianceWeeks *float64 `json:"finishVarianceWeeks"`
	EstimateVariancePct *float64 `json:"estimateVariancePct"`
	BufferUsedPct       *float64 `json:"bufferUsedPct"`
	Status              string   `json:"status"`
	RemainingWeeks      *float64 `json:"remainingWeeks"`
	ForecastFinishWeek  *float64 `json:"forecastFinishWeek"`
	ForecastBasis       string   `json:"forecastBasis"`
}
type InitiativeActual struct {
	IssueCount          int              `json:"issueCount"`
	DoneCount           int              `json:"doneCount"`
	UnknownStatusCount  int              `json:"unknownStatusCount"`
	Name                string           `json:"name"`
	EpicKeys            []string         `json:"epicKeys"`
	Tracked             bool             `json:"tracked"`
	PercentComplete     *float64         `json:"percentComplete"`
	Slices              []SliceActual    `json:"slices"`
	Gaps                []string         `json:"gaps"`
	AddedEpics          []string         `json:"addedEpics"`
	RemovedEpics        []string         `json:"removedEpics"`
	UnplannedPods       []string         `json:"unplannedPods"`
	Suggestions         []EpicSuggestion `json:"suggestions"`
	OriginalScopeSlices []SliceActual    `json:"originalScopeSlices"`
	ActualStartWeek     *float64         `json:"actualStartWeek"`
	ActualFinishWeek    *float64         `json:"actualFinishWeek"`
	StartVarianceWeeks  *float64         `json:"startVarianceWeeks"`
	FinishVarianceWeeks *float64         `json:"finishVarianceWeeks"`
	BufferUsedPct       *float64         `json:"bufferUsedPct"`
	Status              string           `json:"status"`
}
type PodCalibration struct {
	Pod         string  `json:"pod"`
	Factor      float64 `json:"factor"`
	SampleCount int     `json:"sampleCount"`
	Inferred    bool    `json:"inferred"`
}
type ExecutionCoverage struct {
	Total   int `json:"total"`
	Bound   int `json:"bound"`
	Tracked int `json:"tracked"`
}
type OrderAdherence struct {
	Followed       int      `json:"followed"`
	Compared       int      `json:"compared"`
	Percent        *float64 `json:"percent"`
	Inferred       bool     `json:"inferred"`
	OutOfOrderPods []string `json:"outOfOrderPods"`
}
type ExecutionActuals struct {
	Coverage    ExecutionCoverage  `json:"coverage"`
	Initiatives []InitiativeActual `json:"initiatives"`
	Calibration []PodCalibration   `json:"calibration"`
	Adherence   OrderAdherence     `json:"adherence"`
	Gaps        []string           `json:"gaps"`
}

func number(v float64) *float64 { return &v }
func epicIssue(i ExecutionIssue) bool {
	return strings.EqualFold(i.Type, "Epic") || strings.EqualFold(i.Type, "Parent Epic")
}

func knownExecutionStatus(status string) bool {
	return status == "new" || status == "indeterminate" || status == "done"
}
func keyDifference(a, b []string) []string {
	set := map[string]bool{}
	for _, k := range b {
		set[k] = true
	}
	out := []string{}
	for _, k := range a {
		if !set[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
func nameSimilarity(a, b string) float64 {
	words := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') }) {
			out[w] = true
		}
		return out
	}
	x, y := words(a), words(b)
	common := 0
	for w := range x {
		if y[w] {
			common++
		}
	}
	union := len(x) + len(y) - common
	if union == 0 {
		return 0
	}
	return float64(common) / float64(union)
}

// DeriveActuals separates absent evidence from zero variance and scope changes
// from original-scope variance. specs/017-planning-and-execution-usability.md:87
func DeriveActuals(current []Initiative, baseline *BaselineInputs, schedule *Schedule, issues []ExecutionIssue, captured time.Time) ExecutionActuals {
	out := ExecutionActuals{Initiatives: []InitiativeActual{}, Calibration: []PodCalibration{}, Gaps: []string{}, Adherence: OrderAdherence{Inferred: true, OutOfOrderPods: []string{}}}
	out.Coverage.Total = len(current)
	baseInits := map[string]Initiative{}
	scheduled := map[string]ScheduledInitiative{}
	if baseline != nil {
		for _, it := range baseline.Initiatives {
			baseInits[it.Name] = it
		}
	} else {
		out.Gaps = append(out.Gaps, "No active agreed baseline; variance, risk and calibration are unavailable.")
	}
	var origin time.Time
	if schedule != nil {
		origin, _ = time.Parse(isoDate, schedule.PeriodStart)
		for _, it := range schedule.Initiatives {
			scheduled[it.Name] = it
		}
	}
	if origin.IsZero() {
		out.Gaps = append(out.Gaps, "The agreed baseline has no dated period; week-based actuals are unavailable.")
	}
	week := func(t *time.Time) *float64 {
		if t == nil || origin.IsZero() {
			return nil
		}
		return number(t.Sub(origin).Hours() / 168)
	}
	byKey := map[string]ExecutionIssue{}
	for _, issue := range issues {
		byKey[issue.Key] = issue
	}
	belongs := func(issue ExecutionIssue, keys []string) bool {
		bound := map[string]bool{}
		for _, key := range keys {
			bound[key] = true
		}
		seen := map[string]bool{}
		parent := issue.ParentKey
		for parent != "" && !seen[parent] {
			if bound[parent] {
				return true
			}
			seen[parent] = true
			parent = byKey[parent].ParentKey
		}
		return false
	}
	factors := map[string][]float64{}
	perPod := map[string][]SliceActual{}
	for _, it := range current {
		a := InitiativeActual{Name: it.Name, EpicKeys: append([]string{}, it.EpicKeys...), Slices: []SliceActual{}, Gaps: []string{}, AddedEpics: []string{}, RemovedEpics: []string{}, UnplannedPods: []string{}, Suggestions: []EpicSuggestion{}, OriginalScopeSlices: []SliceActual{}, Status: "unknown"}
		base, hasBase := baseInits[it.Name]
		sched, hasSchedule := scheduled[it.Name]
		if hasBase {
			a.AddedEpics = keyDifference(it.EpicKeys, base.EpicKeys)
			a.RemovedEpics = keyDifference(base.EpicKeys, it.EpicKeys)
		}
		if len(it.EpicKeys) == 0 {
			a.Status = "not-tracked"
			a.Gaps = append(a.Gaps, "Not tracked: bind this initiative to Jira epic keys.")
			pods := []string{}
			for pod, work := range it.Work {
				if work.InPath {
					pods = append(pods, pod)
				}
			}
			sort.Strings(pods)
			for _, pod := range pods {
				a.Slices = append(a.Slices, SliceActual{Pod: pod, Confidence: "unknown", Status: "not-tracked", ForecastBasis: "Conditional forecast unavailable: no epic binding.", Gaps: []string{"No epic binding; this planned team has no execution evidence."}})
			}
			if hasBase && len(base.EpicKeys) > 0 {
				original := DeriveActuals([]Initiative{base}, baseline, schedule, issues, captured)
				if len(original.Initiatives) > 0 {
					a.OriginalScopeSlices = original.Initiatives[0].Slices
				}
			}
			for _, issue := range issues {
				if epicIssue(issue) {
					score := nameSimilarity(it.Name, issue.Summary)
					if score >= 0.4 {
						a.Suggestions = append(a.Suggestions, EpicSuggestion{issue.Key, issue.Summary, score})
					}
				}
			}
			sort.Slice(a.Suggestions, func(i, j int) bool {
				if a.Suggestions[i].Score == a.Suggestions[j].Score {
					return a.Suggestions[i].Key < a.Suggestions[j].Key
				}
				return a.Suggestions[i].Score > a.Suggestions[j].Score
			})
			if len(a.Suggestions) > 5 {
				a.Suggestions = a.Suggestions[:5]
			}
			out.Initiatives = append(out.Initiatives, a)
			continue
		}
		out.Coverage.Bound++
		missingEpic := false
		for _, key := range it.EpicKeys {
			if issue, ok := byKey[key]; !ok || !epicIssue(issue) {
				missingEpic = true
				a.Gaps = append(a.Gaps, "Bound epic "+key+" is missing from this snapshot or is not an epic.")
			}
		}
		if missingEpic {
			// A completed captured subset is not completion of the bound scope.
			// specs/017-planning-and-execution-usability.md:193
			a.Gaps = append(a.Gaps, "Bound epic scope is incomplete; observed counts and inferred start are retained, but completion, finish, risk, variance and calibration are unavailable.")
		}
		groups := map[string][]ExecutionIssue{}
		unmappedScope := false
		total, done := 0, 0
		for _, issue := range issues {
			if epicIssue(issue) || !belongs(issue, it.EpicKeys) {
				continue
			}
			total++
			if !knownExecutionStatus(issue.StatusCategory) {
				a.UnknownStatusCount++
			}
			if issue.StatusCategory == "done" {
				done++
			}
			if issue.Pod == "" {
				unmappedScope = true
				a.Gaps = append(a.Gaps, issue.Key+" has no team assignment.")
				continue
			}
			groups[issue.Pod] = append(groups[issue.Pod], issue)
		}
		for pod := range groups {
			found := false
			for _, bs := range sched.Slices {
				if bs.Pod == pod {
					found = true
					break
				}
			}
			if !found {
				unmappedScope = true
			}
		}
		if total > 0 {
			a.Tracked = true
			a.IssueCount, a.DoneCount = total, done
			if a.UnknownStatusCount == 0 && !missingEpic {
				a.PercentComplete = number(float64(done) * 100 / float64(total))
			} else if a.UnknownStatusCount > 0 {
				a.Gaps = append(a.Gaps, "Some captured child issues have unknown status categories; known completed counts are retained, but overall progress and risk are unavailable.")
			}
			out.Coverage.Tracked++
		} else {
			a.Gaps = append(a.Gaps, "No child issue evidence exists for these bindings in the selected snapshot.")
		}
		pods := map[string]bool{}
		for pod := range it.Work {
			if it.Work[pod].InPath {
				pods[pod] = true
			}
		}
		for pod := range groups {
			pods[pod] = true
		}
		names := []string{}
		for pod := range pods {
			names = append(names, pod)
		}
		sort.Strings(names)
		scopeChanged := len(a.AddedEpics) > 0 || len(a.RemovedEpics) > 0
		if scopeChanged && len(base.EpicKeys) > 0 {
			original := base
			comparison := DeriveActuals([]Initiative{original}, baseline, schedule, issues, captured)
			if len(comparison.Initiatives) > 0 {
				a.OriginalScopeSlices = comparison.Initiatives[0].Slices
			}
		}
		for _, pod := range names {
			children := groups[pod]
			slice := SliceActual{Pod: pod, IssueCount: len(children), Confidence: "unknown", Gaps: []string{}, Status: "unknown"}
			var earliest, latest *time.Time
			missingFinish := false
			missingProgress := false
			for _, child := range children {
				if !knownExecutionStatus(child.StatusCategory) {
					missingProgress = true
					slice.UnknownStatusCount++
				}
				if child.StatusCategory == "done" {
					slice.DoneCount++
					if child.Resolved == nil {
						missingFinish = true
					} else if latest == nil || child.Resolved.After(*latest) {
						v := *child.Resolved
						latest = &v
					}
				}
				if child.StatusCategory == "done" || child.StatusCategory == "indeterminate" {
					t := child.Created
					if t == nil {
						t = child.Updated
					}
					if t != nil && (earliest == nil || t.Before(*earliest)) {
						v := *t
						earliest = &v
					}
				}
			}
			switch {
			case missingEpic:
				slice.Gaps = append(slice.Gaps, "Bound epic scope is incomplete; observed team counts and inferred start are retained, but completion, finish, risk, variance, calibration and order comparison are unavailable.")
			case len(children) > 0 && !missingProgress:
				slice.PercentComplete = number(float64(slice.DoneCount) * 100 / float64(len(children)))
				slice.Confidence = "low"
			case missingProgress:
				slice.Gaps = append(slice.Gaps, "Some captured child issues have unknown status categories; known completed counts are retained, but team progress, risk and calibration are unavailable.")
			default:
				slice.Gaps = append(slice.Gaps, "No child issues assigned to this team in the selected snapshot.")
			}
			if earliest != nil {
				slice.StartInferred = true
				slice.ActualStartWeek = week(earliest)
				slice.Gaps = append(slice.Gaps, "No status-transition history; start inferred from earliest created/updated child activity.")
			}
			if len(children) > 0 && slice.DoneCount == len(children) && !missingFinish && !missingEpic {
				slice.ActualFinishWeek = week(latest)
			}
			if missingFinish {
				slice.Gaps = append(slice.Gaps, "A completed issue has no resolution timestamp; finish is unknown.")
			}
			planned := false
			var bs WorkSlice
			for _, s := range sched.Slices {
				if s.Pod == pod {
					bs = s
					planned = true
					break
				}
			}
			if !hasBase || !hasSchedule || !planned {
				if len(children) > 0 {
					a.UnplannedPods = append(a.UnplannedPods, pod)
				}
				slice.Gaps = append(slice.Gaps, "No agreed baseline slice for this team.")
			} else {
				slice.BaselineStartWeek = number(float64(bs.StartWeek))
				slice.BaselineFinishWeek = number(float64(bs.FinishWeek))
				switch {
				case missingEpic:
					slice.Gaps = append(slice.Gaps, "Missing bound epic evidence prevents comparison with the agreed scope.")
				case scopeChanged:
					slice.Gaps = append(slice.Gaps, "Epic scope changed since agreement; combined-scope variance and calibration are withheld.")
				default:
					if slice.ActualStartWeek != nil {
						slice.StartVarianceWeeks = number(*slice.ActualStartWeek - float64(bs.StartWeek))
					}
					if slice.ActualFinishWeek != nil {
						slice.FinishVarianceWeeks = number(*slice.ActualFinishWeek - float64(bs.FinishWeek))
					}
					if !missingProgress && slice.ActualStartWeek != nil && slice.ActualFinishWeek != nil && bs.FinishWeek > bs.StartWeek && *slice.ActualFinishWeek >= *slice.ActualStartWeek {
						factor := (*slice.ActualFinishWeek - *slice.ActualStartWeek) / float64(bs.FinishWeek-bs.StartWeek)
						slice.EstimateVariancePct = number((factor - 1) * 100)
						factors[pod] = append(factors[pod], factor)
					}
					if slice.ActualStartWeek != nil {
						perPod[pod] = append(perPod[pod], slice)
						finish := slice.ActualFinishWeek
						if finish == nil {
							finish = week(&captured)
						}
						if finish != nil && sched.BufferWeeks > 0 && slice.PercentComplete != nil {
							progress := *slice.PercentComplete / 100
							expected := float64(bs.StartWeek) + float64(bs.FinishWeek-bs.StartWeek)*progress
							delay := math.Max(0, *finish-expected)
							slice.BufferUsedPct = number(delay / float64(sched.BufferWeeks) * 100)
							ratio := delay / float64(sched.BufferWeeks) / math.Max(progress, 0.01)
							slice.Status = "on-track"
							if ratio > 0.67 {
								slice.Status = "at-risk"
							}
							if ratio > 1 {
								slice.Status = "late"
							}
						}
					}
				}
			}
			// specs/017-planning-and-execution-usability.md:169: conditional,
			// snapshot-as-of arithmetic, never a live delivery prediction.
			switch {
			case !hasBase || !hasSchedule || !planned:
				slice.ForecastBasis = "Conditional forecast unavailable: no agreed team slice."
			case scopeChanged || unmappedScope:
				slice.ForecastBasis = "Conditional forecast unavailable: changed or unmapped scope is not comparable with agreement."
			case missingEpic:
				slice.ForecastBasis = "Conditional forecast unavailable: bound epic evidence is missing."
			case origin.IsZero() || captured.IsZero() || captured.Unix() <= 0:
				slice.ForecastBasis = "Conditional forecast unavailable: dated baseline period and snapshot timestamp are required."
			case bs.FinishWeek <= bs.StartWeek || (!bs.Estimated && !base.Work[pod].Estimated):
				slice.ForecastBasis = "Conditional forecast unavailable: a positive estimated baseline duration is required."
			case slice.PercentComplete == nil || missingProgress:
				slice.ForecastBasis = "Conditional forecast unavailable: child-issue progress evidence is missing."
			default:
				remaining := float64(bs.FinishWeek-bs.StartWeek) * (1 - *slice.PercentComplete/100)
				slice.RemainingWeeks = number(remaining)
				if slice.DoneCount == slice.IssueCount {
					slice.ForecastBasis = "All captured child issues are complete; zero remaining work. Finish is reported only from resolution timestamps."
				} else {
					snapshotWeek := captured.Sub(origin).Hours() / 168
					slice.ForecastFinishWeek = number(math.Max(snapshotWeek, float64(bs.StartWeek)) + remaining)
					slice.ForecastBasis = "Conditional baseline-rate proxy as of this snapshot: agreed calendar duration × unfinished child-issue fraction, assuming work can continue or start at the agreed rate. Not measured effort, live throughput, a calibrated forecast or a commitment; new blockers and capacity changes are excluded."
				}
			}
			if slice.RemainingWeeks == nil {
				slice.Gaps = append(slice.Gaps, slice.ForecastBasis)
			}
			a.Slices = append(a.Slices, slice)
		}
		allFinished := len(a.Slices) > 0
		assignedCount := 0
		for _, slice := range a.Slices {
			assignedCount += slice.IssueCount
			if slice.ActualStartWeek != nil && (a.ActualStartWeek == nil || *slice.ActualStartWeek < *a.ActualStartWeek) {
				a.ActualStartWeek = number(*slice.ActualStartWeek)
			}
			if slice.ActualFinishWeek == nil {
				allFinished = false
			} else if a.ActualFinishWeek == nil || *slice.ActualFinishWeek > *a.ActualFinishWeek {
				a.ActualFinishWeek = number(*slice.ActualFinishWeek)
			}
		}
		if !allFinished || assignedCount != total || missingEpic {
			a.ActualFinishWeek = nil
		}
		if hasBase && hasSchedule && !scopeChanged && !missingEpic && len(a.UnplannedPods) == 0 && assignedCount == total {
			if a.ActualStartWeek != nil {
				a.StartVarianceWeeks = number(*a.ActualStartWeek - float64(sched.StartWeek))
			}
			if a.ActualFinishWeek != nil {
				a.FinishVarianceWeeks = number(*a.ActualFinishWeek - float64(sched.CommitWeek))
			}
			finish := a.ActualFinishWeek
			if finish == nil && a.ActualStartWeek != nil {
				finish = week(&captured)
			}
			if finish != nil && sched.BufferWeeks > 0 && a.PercentComplete != nil {
				progress := *a.PercentComplete / 100
				expected := float64(sched.StartWeek) + float64(sched.RawFinishWeek-sched.StartWeek)*progress
				burn := math.Max(0, *finish-expected) / float64(sched.BufferWeeks)
				a.Gaps = append(a.Gaps, "Buffer risk uses completed child-issue count as a progress proxy; starts and elapsed-time calibration are inferred, not measured effort.")
				a.BufferUsedPct = number(burn * 100)
				ratio := burn / math.Max(*a.PercentComplete/100, 0.01)
				a.Status = "on-track"
				if ratio > 0.67 {
					a.Status = "at-risk"
				}
				if ratio > 1 {
					a.Status = "late"
				}
			}
		}
		out.Initiatives = append(out.Initiatives, a)
	}
	for pod, values := range factors {
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		out.Calibration = append(out.Calibration, PodCalibration{pod, sum / float64(len(values)), len(values), true})
	}
	sort.Slice(out.Calibration, func(i, j int) bool { return out.Calibration[i].Pod < out.Calibration[j].Pod })
	for pod, slices := range perPod {
		bad := false
		for i := 0; i < len(slices); i++ {
			for j := i + 1; j < len(slices); j++ {
				a, b := slices[i], slices[j]
				expected := *a.BaselineStartWeek - *b.BaselineStartWeek
				if expected == 0 {
					continue
				}
				out.Adherence.Compared++
				if expected*(*a.ActualStartWeek-*b.ActualStartWeek) >= 0 {
					out.Adherence.Followed++
				} else {
					bad = true
				}
			}
		}
		if bad {
			out.Adherence.OutOfOrderPods = append(out.Adherence.OutOfOrderPods, pod)
		}
	}
	sort.Strings(out.Adherence.OutOfOrderPods)
	if out.Adherence.Compared > 0 {
		out.Adherence.Percent = number(float64(out.Adherence.Followed) * 100 / float64(out.Adherence.Compared))
	}
	return out
}
