package main

import (
	"encoding/json"

	"conway/server/db"
)

// The small "dynamic docs" (pods/pod_stats/edges/hygiene) are no longer stored
// as JSON — they're generated from the snapshot tables on read, so Observe,
// compare, Train-seeding and export keep their doc-shaped interface while the
// source of truth is the database. JSON here is only the wire format.

// tableDoc builds one dynamic doc from the snapshot tables. ok=false means the
// snapshot has no table data (a legacy blob snapshot) → caller falls back.
func (s *server) tableDoc(id, path string) ([]byte, bool) {
	pods, err := s.db.Pods(id)
	if err != nil || len(pods) == 0 {
		return nil, false // not a table-backed snapshot
	}
	switch path {
	case "pods.json":
		return podsDocFromRows(pods), true
	case "pod_stats.json":
		st, _ := s.db.PodStats(id)
		return statsDocFromRows(st), true
	case "edges.json":
		e, _ := s.db.Edges(id)
		return edgesDocFromRows(e), true
	case "hygiene.json":
		h, _ := s.db.Hygiene(id)
		return hygieneDocFromRows(h), true
	}
	return nil, false
}

func podsDocFromRows(pods []db.PodRow) []byte {
	type podOut struct {
		Name     string `json:"name"`
		Location string `json:"location"`
		Pairing  bool   `json:"pairing"`
		DevCount int    `json:"devCount"`
		Streams  int    `json:"streams,omitempty"`
		Sre      bool   `json:"sre,omitempty"`
	}
	out := make([]podOut, 0, len(pods))
	for _, p := range pods {
		out = append(out, podOut{p.Name, p.Location, p.Pairing, p.DevCount, p.Streams, p.Sre})
	}
	overlap := map[string]map[string]float64{}
	for _, a := range pods {
		overlap[a.Name] = map[string]float64{}
		for _, b := range pods {
			switch {
			case a.Name == b.Name:
				overlap[a.Name][b.Name] = 8
			case a.Location != "" && a.Location == b.Location:
				overlap[a.Name][b.Name] = 6
			default:
				overlap[a.Name][b.Name] = 2
			}
		}
	}
	doc, _ := json.Marshal(map[string]any{"pods": out, "overlap": overlap})
	return doc
}

func statsDocFromRows(stats []db.PodStatRow) []byte {
	out := map[string]any{}
	for _, s := range stats {
		out[s.Pod] = map[string]any{
			"resolved_count_180d": s.Resolved180d,
			"cycle_time_days":     map[string]float64{"p50": s.P50, "p85": s.P85, "mean": s.Mean},
			"lognormal":           map[string]float64{"mu": s.Mu, "sigma": s.Sigma},
			"wip_count":           s.WipCount,
			"throughput_per_week": s.ThroughputPerWk,
		}
	}
	doc, _ := json.Marshal(out)
	return doc
}

func edgesDocFromRows(edges []db.EdgeRow) []byte {
	type e struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Count int    `json:"count"`
	}
	out := make([]e, 0, len(edges))
	for _, x := range edges {
		out = append(out, e{x.From, x.To, x.Count})
	}
	doc, _ := json.Marshal(out)
	return doc
}

func hygieneDocFromRows(hyg []db.PodHygieneRow) []byte {
	out := map[string]any{}
	for _, h := range hyg {
		out[h.Pod] = map[string]any{
			"sizedPct": h.SizedPct, "sampleSized": h.SampleSized, "medianPoints": h.MedianPoints,
			"staleWipPct": h.StaleWipPct, "unassignedWipPct": h.UnassignedWipPct,
			"wipCount": h.WipCount, "linkDensity": h.LinkDensity, "score": h.Score,
		}
	}
	doc, _ := json.Marshal(out)
	return doc
}
