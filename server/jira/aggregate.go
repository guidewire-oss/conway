// Package jira fetches issues from Jira Cloud and aggregates them into the org
// snapshot documents the engine and Observe consume. The aggregation mirrors
// the offline scripts/aggregate.py so a live import matches the mined baseline.
package jira

import (
	"math"
	"sort"
	"time"
)

// Issue is the minimal shape the aggregation needs from a Jira issue.
type Issue struct {
	Key       string
	Pod       string     // customfield_10026 value (the pod/team)
	IssueType string     // "Epic", "Story", …
	StatusCat string     // statusCategory key: new | indeterminate | done
	ParentKey string     // parent epic's key, "" if none
	Created   time.Time  // zero if unknown
	Resolved  *time.Time // nil if still open
	Blocks    []string   // keys this issue blocks
	BlockedBy []string   // keys that block this issue
}

// WIP counting modes: an in-progress epic and its in-progress children both
// represent the "same" work if counted naively, so a snapshot picks one rule
// at import time.
const (
	// WipModeLeaf counts every non-epic in-progress issue, regardless of
	// whether it has a parent epic. Epics themselves (containers, not units of
	// flow) are never counted. This is the accurate view of concurrent work:
	// an epic with 5 active children is wip=5, not wip=1.
	WipModeLeaf = "leaf"
	// WipModeEpicOrParentless counts an in-progress epic as one unit and does
	// not separately count its children; parentless stories/tasks still count
	// individually.
	WipModeEpicOrParentless = "epic_or_parentless"
)

// PodStat mirrors one entry of pod_stats.json.
type PodStat struct {
	ResolvedCount180d int        `json:"resolved_count_180d"`
	CycleTimeDays     CycleTimes `json:"cycle_time_days"`
	Lognormal         Lognormal  `json:"lognormal"`
	WipCount          int        `json:"wip_count"`
	ThroughputPerWeek float64    `json:"throughput_per_week"`
}

type CycleTimes struct {
	P50  float64 `json:"p50"`
	P85  float64 `json:"p85"`
	Mean float64 `json:"mean"`
}

type Lognormal struct {
	Mu    float64 `json:"mu"`
	Sigma float64 `json:"sigma"`
}

// Edge mirrors one entry of edges.json: a directed cross-pod dependency.
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// podAliases maps Jira pod-field spellings to the org directory's pod names.
var podAliases = map[string]string{"Moose Factory": "MooseFactory"}

func podOf(raw string) string {
	if a, ok := podAliases[raw]; ok {
		return a
	}
	return raw
}

const (
	maxCycleDays = 180.0
	excludeEpic  = "Epic"
	excludeEpic2 = "Parent Epic"
	weeks180     = 26.0 // ~180 days, matching aggregate.py's throughput window
)

// Aggregate turns issues into per-pod stats and cross-pod edges, reproducing
// scripts/aggregate.py: epics and >180d backlog artifacts are dropped from
// cycle time, the rest winsorized at the pod's p95; edges are deduped
// (blocker,blocked) pairs across different pods. wipMode picks how epics vs.
// their children are counted toward wip (see WipModeLeaf / WipModeEpicOrParentless).
func Aggregate(issues []Issue, wipMode string) (map[string]PodStat, []Edge) {
	podByKey := map[string]string{}
	for _, it := range issues {
		if p := podOf(it.Pod); p != "" {
			podByKey[it.Key] = p
		}
	}

	// dedup (blocker, blocked) pairs -> cross-pod edge counts
	pairs := map[[2]string]bool{}
	for _, it := range issues {
		for _, k := range it.Blocks {
			pairs[[2]string{it.Key, k}] = true
		}
		for _, k := range it.BlockedBy {
			pairs[[2]string{k, it.Key}] = true
		}
	}
	edgeCount := map[[2]string]int{}
	for pair := range pairs {
		pa, pb := podByKey[pair[0]], podByKey[pair[1]]
		if pa == "" || pb == "" || pa == pb {
			continue
		}
		edgeCount[[2]string{pa, pb}]++
	}
	edges := make([]Edge, 0, len(edgeCount))
	for k, c := range edgeCount {
		edges = append(edges, Edge{From: k[0], To: k[1], Count: c})
	}
	// deterministic: count desc, then names
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Count != edges[j].Count {
			return edges[i].Count > edges[j].Count
		}
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	// cycle-time samples per pod
	byPod := map[string][]float64{}
	for _, it := range issues {
		p := podOf(it.Pod)
		if p == "" || it.Resolved == nil || it.Created.IsZero() {
			continue
		}
		if it.IssueType == excludeEpic || it.IssueType == excludeEpic2 {
			continue
		}
		days := it.Resolved.Sub(it.Created).Hours() / 24
		if days < 0.25 {
			days = 0.25
		}
		if days > maxCycleDays {
			continue
		}
		byPod[p] = append(byPod[p], days)
	}
	// winsorize each pod at its p95 (index int(0.95*(n-1)), matching aggregate.py)
	for p, xs := range byPod {
		sort.Float64s(xs)
		cap := xs[int(0.95*float64(len(xs)-1))]
		for i := range xs {
			if xs[i] > cap {
				xs[i] = cap
			}
		}
		byPod[p] = xs
	}

	// wip = actively in progress (status category "indeterminate"), matching
	// the drill-down's definition (server/db/snapshotquery.go WipPage/WipSummary)
	// — an unresolved backlog issue is not wip just because it's not done yet.
	wipByPod := map[string]int{}
	for _, it := range issues {
		if it.Resolved != nil || it.StatusCat != "indeterminate" {
			continue
		}
		isEpic := it.IssueType == excludeEpic || it.IssueType == excludeEpic2
		counts := !isEpic // WipModeLeaf: every non-epic issue
		if wipMode == WipModeEpicOrParentless {
			counts = isEpic || it.ParentKey == ""
		}
		if !counts {
			continue
		}
		if p := podOf(it.Pod); p != "" {
			wipByPod[p]++
		}
	}

	stats := map[string]PodStat{}
	for p, xs := range byPod {
		logs := make([]float64, len(xs))
		var sum, lsum float64
		for i, x := range xs {
			logs[i] = math.Log(x)
			lsum += logs[i]
			sum += x
		}
		mu := lsum / float64(len(logs))
		var varSum float64
		for _, l := range logs {
			varSum += (l - mu) * (l - mu)
		}
		sigma := math.Sqrt(varSum / float64(len(logs)))
		if sigma == 0 {
			sigma = 0.1
		}
		stats[p] = PodStat{
			ResolvedCount180d: len(xs),
			CycleTimeDays:     CycleTimes{P50: round2(pct(xs, 50)), P85: round2(pct(xs, 85)), Mean: round2(sum / float64(len(xs)))},
			Lognormal:         Lognormal{Mu: round4(mu), Sigma: round4(sigma)},
			WipCount:          wipByPod[p],
			ThroughputPerWeek: round2(float64(len(xs)) / weeks180),
		}
	}
	// pods with WIP but nothing resolved still deserve an entry
	for p, w := range wipByPod {
		if _, ok := stats[p]; ok {
			continue
		}
		stats[p] = PodStat{
			ResolvedCount180d: 0,
			CycleTimeDays:     CycleTimes{P50: 7.0, P85: 17.0, Mean: 10.0},
			Lognormal:         Lognormal{Mu: math.Log(7), Sigma: 0.9},
			WipCount:          w,
			ThroughputPerWeek: 0.0,
		}
	}
	return stats, edges
}

// pct returns the linearly-interpolated q-th percentile of xs (xs is sorted).
func pct(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	idx := (q / 100) * float64(len(xs)-1)
	lo, hi := math.Floor(idx), math.Ceil(idx)
	return xs[int(lo)] + (xs[int(hi)]-xs[int(lo)])*(idx-lo)
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
func round4(x float64) float64 { return math.Round(x*10000) / 10000 }
