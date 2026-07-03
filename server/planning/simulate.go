package planning

import (
	"math"
	"sort"
)

// The simulation is a decision aid, not a forecast. Utilization (ρ) is the
// primary, defensible signal; lead time is directional. queueMult turns "how
// full a pod is" into "how much longer work waits": m(ρ)=1/(1−ρ), clamped at
// ρ=0.95 (beyond that it's "won't fit", not a precise number).
func queueMult(rho float64) float64 {
	r := rho
	if r < 0 {
		r = 0
	}
	if r > 0.95 {
		r = 0.95
	}
	return 1 / (1 - r)
}

// Lever is one what-if adjustment.
type Lever struct {
	Type       string  `json:"type"`       // addCapacity | unpair | descope | defer | reduceWip | reassign | dropPod
	Pod        string  `json:"pod"`        // addCapacity, unpair, reassign (from), dropPod
	ToPod      string  `json:"toPod"`      // reassign (to)
	Initiative string  `json:"initiative"` // descope, defer, dropPod (optional: all initiatives if empty)
	N          float64 `json:"n"`          // addCapacity: +tracks; descope/reduceWip: fraction 0..1
}

type InitiativeResult struct {
	Name       string  `json:"name"`
	LeadWeeks  float64 `json:"leadWeeks"`
	Bottleneck string  `json:"bottleneck"`
	Fits       bool    `json:"fits"`
}

type SimResult struct {
	Loads           []PodLoad          `json:"loads"`
	Initiatives     []InitiativeResult `json:"initiatives"`
	Constraints     int                `json:"constraints"`
	Fitting         int                `json:"fitting"`
	Total           int                `json:"total"`
	MedianLeadWeeks float64            `json:"medianLeadWeeks"`
}

func clampFrac(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// ComputeResult derives utilization and (directional) lead times. wipGain models
// a WIP-limit / focus lever as recovered multitasking waste, reducing effective
// demand.
func ComputeResult(teams []Team, inits []Initiative, params Params, wipGain float64) SimResult {
	params = params.WithDefaults()
	if wipGain > 0.5 {
		wipGain = 0.5
	}
	if wipGain < 0 {
		wipGain = 0
	}
	tracks := map[string]int{}
	names := map[string]bool{}
	for _, t := range teams {
		tracks[t.Name] = t.EffectiveTracks()
		names[t.Name] = true
	}
	demand := map[string]float64{}
	for _, it := range inits {
		for tm, wk := range it.Work {
			if wk.InPath {
				demand[tm] += wk.Weeks
				names[tm] = true
			}
		}
	}
	rho := map[string]float64{}
	var loads []PodLoad
	for name := range names {
		d := demand[name] * (1 - wipGain)
		tr := tracks[name]
		capw := float64(tr) * params.HorizonWeeks * (1 - params.CapacityLoss)
		var r float64
		switch {
		case capw > 0:
			r = d / capw
		case d > 0:
			r = InfiniteRho
		}
		rho[name] = r
		loads = append(loads, PodLoad{Team: name, DemandWeeks: d, Tracks: tr, CapacityWeeks: capw, Rho: r, Constraint: r >= 1})
	}
	sort.Slice(loads, func(i, j int) bool { return loads[i].Rho > loads[j].Rho })

	var irs []InitiativeResult
	var leads []float64
	fitting, constraints := 0, 0
	for _, l := range loads {
		if l.Constraint {
			constraints++
		}
	}
	for _, it := range inits {
		lead, bn := initiativeLead(it, rho)
		fits := lead <= params.HorizonWeeks
		if fits {
			fitting++
		}
		irs = append(irs, InitiativeResult{Name: it.Name, LeadWeeks: round1(lead), Bottleneck: bn, Fits: fits})
		leads = append(leads, lead)
	}
	return SimResult{Loads: loads, Initiatives: irs, Constraints: constraints,
		Fitting: fitting, Total: len(inits), MedianLeadWeeks: round1(median(leads))}
}

// initiativeLead is the critical path through the initiative's dependency DAG;
// each pod contributes weeks × m(ρ). Returns lead time and the highest-ρ pod.
func initiativeLead(it Initiative, rho map[string]float64) (float64, string) {
	inwork := map[string]bool{}
	for tm, wk := range it.Work {
		if wk.InPath {
			inwork[tm] = true
		}
	}
	memo := map[string]float64{}
	visiting := map[string]bool{}
	var leadAt func(pod string) float64
	leadAt = func(pod string) float64 {
		if v, ok := memo[pod]; ok {
			return v
		}
		if visiting[pod] {
			return 0 // cycle guard
		}
		visiting[pod] = true
		wk := it.Work[pod]
		cost := wk.Weeks * queueMult(rho[pod])
		maxDep := 0.0
		for _, dep := range wk.DependsOn {
			if inwork[dep] {
				if l := leadAt(dep); l > maxDep {
					maxDep = l
				}
			}
		}
		visiting[pod] = false
		memo[pod] = cost + maxDep
		return memo[pod]
	}
	var lead float64
	bn, bnRho := "", -1.0
	for pod := range inwork {
		if l := leadAt(pod); l > lead {
			lead = l
		}
		if r := rho[pod]; r > bnRho {
			bnRho, bn = r, pod
		}
	}
	return lead, bn
}

// Simulate returns the baseline and the levered ("after") result.
func Simulate(teams []Team, inits []Initiative, params Params, levers []Lever) (before, after SimResult) {
	before = ComputeResult(teams, inits, params, 0)
	t2, i2, wip := applyLevers(teams, inits, levers)
	after = ComputeResult(t2, i2, params, wip)
	return
}

func applyLevers(teams []Team, inits []Initiative, levers []Lever) ([]Team, []Initiative, float64) {
	t2 := append([]Team(nil), teams...) // Team has no pointer fields; value copy is a safe clone
	i2 := make([]Initiative, 0, len(inits))
	for _, it := range inits {
		w := make(map[string]TeamWork, len(it.Work))
		for k, v := range it.Work {
			w[k] = v
		}
		i2 = append(i2, Initiative{Name: it.Name, Description: it.Description, Leads: it.Leads, Work: w})
	}
	teamIdx := func(name string) int {
		for i := range t2 {
			if t2[i].Name == name {
				return i
			}
		}
		return -1
	}
	wipGain := 0.0
	for _, lv := range levers {
		switch lv.Type {
		case "addCapacity":
			if i := teamIdx(lv.Pod); i >= 0 {
				t2[i].Tracks = t2[i].EffectiveTracks() + int(lv.N)
			}
		case "unpair":
			if i := teamIdx(lv.Pod); i >= 0 {
				t2[i].Tracks = t2[i].EffectiveTracks() * 2
				t2[i].Pairs = false
			}
		case "descope":
			for j := range i2 {
				if i2[j].Name == lv.Initiative {
					for k, v := range i2[j].Work {
						v.Weeks *= (1 - clampFrac(lv.N))
						i2[j].Work[k] = v
					}
				}
			}
		case "defer":
			kept := i2[:0:0]
			for _, it := range i2 {
				if it.Name != lv.Initiative {
					kept = append(kept, it)
				}
			}
			i2 = kept
		case "reduceWip":
			wipGain += clampFrac(lv.N)
		case "reassign": // move a pod's work (and dep references) to another pod
			from, to := lv.Pod, lv.ToPod
			if from == "" || to == "" || from == to {
				break
			}
			for j := range i2 {
				if wk, ok := i2[j].Work[from]; ok {
					dst := i2[j].Work[to]
					dst.Weeks += wk.Weeks
					dst.InPath = true
					dst.Estimated = dst.Estimated || wk.Estimated
					dst.DependsOn = mergeDeps(dst.DependsOn, wk.DependsOn, to)
					i2[j].Work[to] = dst
					delete(i2[j].Work, from)
				}
				for tm, w := range i2[j].Work {
					w.DependsOn = renameDep(w.DependsOn, from, to, tm)
					i2[j].Work[tm] = w
				}
			}
		case "dropPod": // remove a pod's slice (from one initiative, or all)
			for j := range i2 {
				if lv.Initiative != "" && i2[j].Name != lv.Initiative {
					continue
				}
				delete(i2[j].Work, lv.Pod)
				for tm, w := range i2[j].Work {
					w.DependsOn = renameDep(w.DependsOn, lv.Pod, "", tm)
					i2[j].Work[tm] = w
				}
			}
		}
	}
	return t2, i2, wipGain
}

// mergeDeps unions two dependency lists, dropping blanks, self, and duplicates.
func mergeDeps(a, b []string, self string) []string {
	seen := map[string]bool{self: true, "": true}
	var out []string
	for _, d := range append(append([]string{}, a...), b...) {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// renameDep replaces `from` with `to` in a dep list (to=="" drops it), dropping
// self and duplicates.
func renameDep(deps []string, from, to, self string) []string {
	seen := map[string]bool{self: true, "": true}
	var out []string
	for _, d := range deps {
		if d == from {
			d = to
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}

func round1(f float64) float64 {
	if math.IsInf(f, 1) {
		return f
	}
	return math.Round(f*10) / 10
}
