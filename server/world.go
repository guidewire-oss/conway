package main

import (
	"conway/server/game"
)

// World is the org snapshot the engine builds games from — always sourced
// from a Postgres snapshot (see worldFromSnapshot in snapshots.go); there is
// no local-file fallback, so a database is required to run games.
type World struct {
	Pods    []game.PodInfo
	Stats   map[string]game.PodStat
	Overlap map[string]map[string]float64
	Edges   []*game.Edge
	SrePods map[string]bool
}

// buildWorld parses a world from a document reader — the JSON shapes a
// snapshot's dynamic docs are generated in (see tableDoc in snapshotdocs.go).
func buildWorld(read func(name string, v any) error) (*World, error) {
	var podsFile struct {
		Pods []struct {
			Name     string  `json:"name"`
			Location string  `json:"location"`
			Pairing  bool    `json:"pairing"`
			DevCount float64 `json:"devCount"`
			Streams  float64 `json:"streams"` // explicit work-streams (pairs); 0 = derive
			Sre      bool    `json:"sre"`     // SRE/platform-reliability pod (org-specific — set per pod in pods.json)
		} `json:"pods"`
		Overlap map[string]map[string]float64 `json:"overlap"`
	}
	if err := read("pods.json", &podsFile); err != nil {
		return nil, err
	}

	var stats map[string]struct {
		Resolved   float64                          `json:"resolved_count_180d"`
		Cycle      struct{ P50, P85, Mean float64 } `json:"cycle_time_days"`
		Lognormal  struct{ Mu, Sigma float64 }      `json:"lognormal"`
		Wip        float64                          `json:"wip_count"`
		Throughput float64                          `json:"throughput_per_week"`
	}
	if err := read("pod_stats.json", &stats); err != nil {
		return nil, err
	}

	var edgesRaw []struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Count int    `json:"count"`
	}
	if err := read("edges.json", &edgesRaw); err != nil {
		return nil, err
	}

	var hyg map[string]struct {
		Score float64 `json:"score"`
	}
	_ = read("hygiene.json", &hyg) // optional

	w := &World{Overlap: podsFile.Overlap, Stats: map[string]game.PodStat{}, SrePods: map[string]bool{}}
	for _, p := range podsFile.Pods {
		w.Pods = append(w.Pods, game.PodInfo{Name: p.Name, Location: p.Location, Pairing: p.Pairing, DevCount: p.DevCount, Streams: p.Streams})
		if p.Sre {
			w.SrePods[p.Name] = true
		}
		s := stats[p.Name]
		// rho0 proxy mirrors the live app (wip / (streams*2)); engine clamps load itself.
		// An explicit work-streams count (e.g. # pairs) wins over the devCount/pairing guess.
		streams := p.Streams
		if streams == 0 {
			streams = p.DevCount
			if p.Pairing {
				streams = p.DevCount / 2
			}
		}
		rho0 := 0.5
		if streams > 0 && s.Wip > 0 {
			rho0 = s.Wip / (streams * 2)
		}
		hygiene := 0.5
		if h, ok := hyg[p.Name]; ok && h.Score > 0 {
			hygiene = h.Score
		}
		ps := game.PodStat{
			Wip: s.Wip, ThroughputWk: s.Throughput, Mu: s.Lognormal.Mu, Sigma: s.Lognormal.Sigma,
			Rho0: rho0, P50: s.Cycle.P50, P85: s.Cycle.P85, HygieneScore: hygiene,
		}
		if ps.ThroughputWk == 0 {
			ps.ThroughputWk = 2
		} // synthetic fallback
		if ps.Wip == 0 {
			ps.Wip = p.DevCount
		}
		w.Stats[p.Name] = ps
	}
	for _, e := range edgesRaw {
		if _, ok := w.Stats[e.From]; !ok {
			continue
		}
		if _, ok := w.Stats[e.To]; !ok {
			continue
		}
		w.Edges = append(w.Edges, &game.Edge{From: e.From, To: e.To, Count: e.Count})
	}
	return w, nil
}

// freshEdges clones the world edges so each game mutates its own copy.
func (w *World) freshEdges() []*game.Edge {
	out := make([]*game.Edge, len(w.Edges))
	for i, e := range w.Edges {
		c := *e
		out[i] = &c
	}
	return out
}
