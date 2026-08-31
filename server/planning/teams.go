package planning

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Team is a pod from the uploaded roster (pod-directory CSV/XLSX). Capacity is
// measured in parallel work-tracks (servers), not headcount: a pairing team
// runs ~ceil(devs/2) tracks.
type Team struct {
	Name   string `json:"name"`
	Devs   int    `json:"devs"`           // developer headcount (from the Developers column)
	Pairs  bool   `json:"pairs"`          // does this team pair-program?
	Tracks int    `json:"tracks"`         // explicit track override (0 = derive from devs/pairs)
	Site   string `json:"site,omitempty"` // location, for cross-site-seam analysis
	// CapacityLoss (spec 014): this pod's own fraction of tracked time that
	// never becomes product work (ops burden, support, on-call, ramp). 0 means
	// inherit the plan's global loss — the override is opt-in per pod.
	CapacityLoss float64 `json:"capacityLoss,omitempty"`
}

// EffectiveLoss is the loss this pod plans with: its own override when one is
// set, else the plan's global figure. The one definition of pod loss —
// durations, placement, utilization, and the fit line all go through here, so
// the views cannot disagree (spec 014 FR-002).
func (t Team) EffectiveLoss(global float64) float64 {
	if t.CapacityLoss > 0 {
		return t.CapacityLoss
	}
	return global
}

// EffectiveTracks is the pod's max parallel tracks of work: an explicit override
// wins; else pairing halves the dev count (rounding up so a lone dev still gets
// a track); else one track per developer.
func (t Team) EffectiveTracks() int {
	if t.Tracks > 0 {
		return t.Tracks
	}
	if t.Pairs {
		if t.Devs <= 0 {
			return 0
		}
		return (t.Devs + 1) / 2
	}
	return t.Devs
}

// Params are the plan-level capacity assumptions.
type Params struct {
	HorizonWeeks float64 `json:"horizonWeeks"` // length of the planning period
	CapacityLoss float64 `json:"capacityLoss"` // fraction lost to PTO/attrition/ramp (0..1)
}

// WithDefaults fills unset params: 26-week half-year horizon, 10% capacity loss.
func (p Params) WithDefaults() Params {
	if p.HorizonWeeks <= 0 {
		p.HorizonWeeks = 26
	}
	if p.CapacityLoss < 0 || p.CapacityLoss >= 1 {
		p.CapacityLoss = 0.10
	}
	return p
}

// ReadGrid reads an upload into a dense grid: .xlsx (ZIP magic "PK", worksheet
// matched by sheetMatch) or CSV. One entry point for both upload kinds.
func ReadGrid(data []byte, sheetMatch string) ([][]string, error) {
	if len(data) >= 2 && data[0] == 'P' && data[1] == 'K' {
		return ReadXLSX(data, sheetMatch)
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	return r.ReadAll()
}

// ParseTeamsCSV parses a pod-directory CSV (handles quoted Developers lists).
func ParseTeamsCSV(data []byte) ([]Team, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // rows can have ragged lengths
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	return ParseTeamsRows(rows)
}

// ParseTeamsRows builds the roster from a dense grid (row 0 = header), shared by
// the CSV and XLSX paths. It auto-detects the Name / Developers / Location /
// Pairs / Tracks / Capacity Loss columns by header. An out-of-range loss cell
// refuses the whole roster, naming the pod (spec 014 AC 2.2) — a silently
// clamped loss would under-plan one pod while looking fine everywhere else.
func ParseTeamsRows(rows [][]string) ([]Team, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	idx := map[string]int{"name": -1, "devs": -1, "site": -1, "pairs": -1, "tracks": -1, "loss": -1}
	for i, h := range rows[0] {
		l := strings.ToLower(strings.TrimSpace(h))
		switch {
		case idx["name"] < 0 && (l == "pod name" || l == "team" || l == "pod" || l == "name"):
			idx["name"] = i
		case idx["devs"] < 0 && strings.HasPrefix(l, "developer"):
			idx["devs"] = i
		case idx["site"] < 0 && (l == "location" || l == "site"):
			idx["site"] = i
		case idx["pairs"] < 0 && strings.HasPrefix(l, "pair"):
			idx["pairs"] = i
		case idx["tracks"] < 0 && (l == "tracks" || l == "capacity" || strings.Contains(l, "stream") || strings.Contains(l, "parallel") || strings.Contains(l, "lane")):
			idx["tracks"] = i
		case idx["loss"] < 0 && strings.Contains(l, "loss"):
			idx["loss"] = i
		}
	}
	if idx["name"] < 0 {
		idx["name"] = 0
	}
	at := func(row []string, key string) string {
		if i := idx[key]; i >= 0 && i < len(row) {
			return row[i]
		}
		return ""
	}
	nameCol := idx["name"]
	var teams []Team
	for _, row := range rows[1:] {
		name := ""
		if nameCol < len(row) {
			name = strings.TrimSpace(row[nameCol])
		}
		if name == "" {
			continue
		}
		t := Team{
			Name:  name,
			Devs:  countPeople(at(row, "devs")),
			Pairs: truthy(at(row, "pairs")),
			Site:  strings.TrimSpace(at(row, "site")),
		}
		if n, err := strconv.Atoi(strings.TrimSpace(at(row, "tracks"))); err == nil && n > 0 {
			t.Tracks = n
		}
		// The loss cell reads as a percent ("15", "15%") or a fraction ("0.15");
		// empty inherits the plan global (spec 014 AC 2.1). Exactly 100% would
		// leave the pod no capacity at all, so it is refused like any other
		// out-of-range value. A percent-suffixed cell divides by 100
		// unconditionally — "0.5%" is half a percent, not fifty.
		trimmed := strings.TrimSpace(at(row, "loss"))
		pct := strings.HasSuffix(trimmed, "%")
		raw := strings.TrimSpace(strings.TrimSuffix(trimmed, "%"))
		if raw != "" {
			v, err := strconv.ParseFloat(raw, 64)
			// ParseFloat accepts "nan" and NaN fails every range comparison,
			// so it is refused explicitly — a NaN loss would marshal-fail and
			// silently wipe the roster (spec 014 review).
			if err != nil || math.IsNaN(v) {
				return nil, fmt.Errorf("pod %q: capacity loss %q must be a percent between 0 and 100", name, raw)
			}
			if pct || v > 1 {
				v /= 100 // a bare number reads as a percent
			}
			if v < 0 || v >= 1 {
				return nil, fmt.Errorf("pod %q: capacity loss %q must be a percent between 0 and 100", name, raw)
			}
			t.CapacityLoss = v
		}
		teams = append(teams, t)
	}
	return teams, nil
}

// countPeople returns the headcount in a Developers cell: a plain integer is
// taken as the count directly ("8" -> 8); otherwise it counts entries in a
// comma/semicolon/newline-separated people list ("Ann, Bob" -> 2).
func countPeople(cell string) int {
	s := strings.TrimSpace(cell)
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	n := 0
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		if strings.TrimSpace(part) != "" {
			n++
		}
	}
	return n
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "true", "1", "pairs", "pairing":
		return true
	}
	return false
}

// InfiniteRho stands in for a literal +Inf (demand but zero capacity) — plain
// encoding/json cannot marshal Inf/NaN, and a silently-dropped response is
// worse than a very large finite number here. queueMult clamps rho at 0.95
// regardless, so this has no effect on lead-time math.
const InfiniteRho = 1e9

// PodLoad is one pod's demand vs capacity over the planning horizon.
type PodLoad struct {
	Team          string  `json:"team"`
	DemandWeeks   float64 `json:"demandWeeks"`   // sum of estimates assigned to the pod
	Tracks        int     `json:"tracks"`        // effective parallel tracks
	CapacityWeeks float64 `json:"capacityWeeks"` // tracks * horizon * (1 - effective loss)
	Rho           float64 `json:"rho"`           // demand / capacity (utilization)
	Constraint    bool    `json:"constraint"`    // rho >= 1: over capacity for the period
	LossPct       float64 `json:"lossPct"`       // the pod's effective loss, in percent (spec 014)
	LossOverride  bool    `json:"lossOverride"`  // true when the pod sets its own loss, not the plan's
}

// Utilization computes per-pod demand, capacity, and ρ for the plan. Pods are
// returned hottest-first. Pods present in the plan but not the roster get zero
// tracks (flagged as a constraint if they carry demand).
func Utilization(plan *Plan, teams []Team, params Params) []PodLoad {
	params = params.WithDefaults()
	tracks := map[string]int{}
	for _, t := range teams {
		tracks[t.Name] = t.EffectiveTracks()
	}
	demand := map[string]float64{}
	for _, init := range plan.Initiatives {
		for team, w := range init.Work {
			if w.InPath {
				demand[team] += w.Weeks
			}
		}
	}
	// union of teams that have a roster entry or any demand
	names := map[string]bool{}
	byName := map[string]Team{}
	for _, t := range teams {
		names[t.Name] = true
		byName[t.Name] = t
	}
	for team := range demand {
		names[team] = true
	}
	var out []PodLoad
	for name := range names {
		tr := tracks[name]
		// The pod's own loss, inheriting the plan global when unset (spec 014
		// FR-002/FR-003): a demand-only pod with no roster entry inherits too.
		loss := byName[name].EffectiveLoss(params.CapacityLoss)
		capw := float64(tr) * params.HorizonWeeks * (1 - loss)
		d := demand[name]
		pl := PodLoad{Team: name, DemandWeeks: d, Tracks: tr, CapacityWeeks: capw}
		switch {
		case capw > 0:
			pl.Rho = d / capw
		case d > 0:
			pl.Rho = InfiniteRho // demand but no capacity
		}
		pl.Constraint = pl.Rho >= 1
		pl.LossPct = math.Round(loss*1000) / 10
		pl.LossOverride = byName[name].CapacityLoss > 0
		out = append(out, pl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rho > out[j].Rho })
	return out
}
