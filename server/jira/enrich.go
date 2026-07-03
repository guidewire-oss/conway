package jira

import (
	"sort"
	"time"
)

// HygieneStat is one pod's data-quality aggregate (typed, for the
// snapshot_pod_hygiene table). Pointer fields are nil when not computable.
type HygieneStat struct {
	Pod                                                   string
	SizedPct, MedianPoints, StaleWipPct, UnassignedWipPct *float64
	LinkDensity, Score                                    *float64
	SampleSized, WipCount                                 int
}

// HygieneStats computes per-pod data-quality aggregates from the issues (the
// same rules as the old hygiene.json, returned typed for DB storage).
func HygieneStats(issues []DetailedIssue, now time.Time) []HygieneStat {
	const staleD = 14.0
	type acc struct{ sized, total, staleWip, wip, unassigned int }
	pa := map[string]*acc{}
	pts := map[string][]float64{}
	linked := map[string]map[string]bool{}
	resolved := map[string]int{}
	get := func(p string) *acc {
		if pa[p] == nil {
			pa[p] = &acc{}
		}
		return pa[p]
	}
	seen := map[string]bool{}
	for _, it := range issues {
		p := podOf(it.Pod)
		if p == "" {
			continue
		}
		if it.IssueType != excludeEpic && it.IssueType != excludeEpic2 && !seen[it.Key] {
			seen[it.Key] = true
			a := get(p)
			a.total++
			if it.Points != nil {
				a.sized++
				pts[p] = append(pts[p], *it.Points)
			}
		}
		if it.InProgress() {
			a := get(p)
			a.wip++
			if !it.Updated.IsZero() && now.Sub(it.Updated).Hours()/24 > staleD {
				a.staleWip++
			}
			if it.Assignee == "" {
				a.unassigned++
			}
		}
		if len(it.Blocks) > 0 || len(it.BlockedBy) > 0 {
			if linked[p] == nil {
				linked[p] = map[string]bool{}
			}
			linked[p][it.Key] = true
		}
		if it.Resolved != nil {
			resolved[p]++
		}
	}
	pods := map[string]bool{}
	for p := range pa {
		pods[p] = true
	}
	for p := range linked {
		pods[p] = true
	}
	for p := range resolved {
		pods[p] = true
	}
	out := make([]HygieneStat, 0, len(pods))
	for p := range pods {
		a := get(p)
		h := HygieneStat{Pod: p, SampleSized: a.total, WipCount: a.wip}
		var comps []float64
		if a.total > 0 {
			sp := round3(float64(a.sized) / float64(a.total))
			h.SizedPct = &sp
			comps = append(comps, sp)
		}
		if len(pts[p]) > 0 {
			mp := median(pts[p])
			h.MedianPoints = &mp
		}
		if a.wip > 0 {
			st := round3(float64(a.staleWip) / float64(a.wip))
			un := round3(float64(a.unassigned) / float64(a.wip))
			h.StaleWipPct, h.UnassignedWipPct = &st, &un
			comps = append(comps, 1-st, 1-un)
		}
		if resolved[p] > 0 {
			ld := round3(float64(len(linked[p])) / float64(resolved[p]))
			h.LinkDensity = &ld
		}
		if len(comps) > 0 {
			var s float64
			for _, c := range comps {
				s += c
			}
			sc := round3(s / float64(len(comps)))
			h.Score = &sc
		}
		out = append(out, h)
	}
	return out
}

func round3(x float64) float64 { return float64(int(x*1000+0.5)) / 1000 }

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
