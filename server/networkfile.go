package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
)

// NetworkFile is the human-editable representation of an org network — the
// download/upload format facilitators use to author scenarios. It round-trips
// with a snapshot's world documents (pods.json/pod_stats.json/edges.json/
// hygiene.json) but uses friendly field names. stats/overlap are optional.
type NetworkFile struct {
	Name    string                        `json:"name"`
	Pods    []NetPod                      `json:"pods"`
	Edges   []NetEdge                     `json:"edges"`
	Stats   map[string]NetStat            `json:"stats,omitempty"`
	Overlap map[string]map[string]float64 `json:"overlap,omitempty"`
}

type NetPod struct {
	Name     string `json:"name"`
	Location string `json:"location"`
	Pairing  bool   `json:"pairing"`
	DevCount int    `json:"devCount"`
	Streams  int    `json:"streams,omitempty"` // explicit work-streams (pairs); 0 = derive from devCount/pairing
}

type NetEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

type NetStat struct {
	Wip               float64 `json:"wip"`
	ThroughputPerWeek float64 `json:"throughputPerWeek"`
	CycleP50          float64 `json:"cycleP50"`
	CycleP85          float64 `json:"cycleP85"`
	Hygiene           float64 `json:"hygiene"`
}

// --- the internal world-doc shapes (mirror the snapshot docs from
// snapshotdocs.go and world.go's buildWorld) ---

type podsDoc struct {
	Pods []struct {
		Name     string  `json:"name"`
		Location string  `json:"location"`
		Pairing  bool    `json:"pairing"`
		DevCount float64 `json:"devCount"`
		Streams  float64 `json:"streams"`
	} `json:"pods"`
	Overlap map[string]map[string]float64 `json:"overlap"`
}

type statDoc struct {
	Resolved   int                              `json:"resolved_count_180d"`
	Cycle      struct{ P50, P85, Mean float64 } `json:"cycle_time_days"`
	Lognormal  struct{ Mu, Sigma float64 }      `json:"lognormal"`
	Wip        float64                          `json:"wip_count"`
	Throughput float64                          `json:"throughput_per_week"`
}

// snapshotToNetwork assembles the editable NetworkFile from a snapshot's docs.
func (s *server) snapshotToNetwork(id, name string) (*NetworkFile, error) {
	read := func(path string, v any) {
		if b, ok := s.tableDoc(id, path); ok { // table-backed snapshot
			json.Unmarshal(b, v)
			return
		}
		if b, _ := s.db.GetSnapshotDoc(id, path); b != nil { // legacy blob
			json.Unmarshal(b, v)
		}
	}
	var pods podsDoc
	read("pods.json", &pods)
	if len(pods.Pods) == 0 {
		return nil, fmt.Errorf("snapshot has no pods")
	}
	var stats map[string]statDoc
	read("pod_stats.json", &stats)
	var edges []NetEdge
	read("edges.json", &edges)
	var hyg map[string]struct {
		Score float64 `json:"score"`
	}
	read("hygiene.json", &hyg)

	nf := &NetworkFile{Name: name, Overlap: pods.Overlap}
	for _, p := range pods.Pods {
		nf.Pods = append(nf.Pods, NetPod{Name: p.Name, Location: p.Location, Pairing: p.Pairing, DevCount: int(p.DevCount + 0.5), Streams: int(p.Streams + 0.5)})
	}
	if len(stats) > 0 {
		nf.Stats = map[string]NetStat{}
		for name, st := range stats {
			nf.Stats[name] = NetStat{
				Wip: st.Wip, ThroughputPerWeek: st.Throughput,
				CycleP50: st.Cycle.P50, CycleP85: st.Cycle.P85, Hygiene: hyg[name].Score,
			}
		}
	}
	nf.Edges = edges
	return nf, nil
}

// networkToData converts an edited NetworkFile into snapshot table rows (a
// template/edited scenario has structure + synthesized stats, but no issues).
func networkToData(nf NetworkFile) (db.SnapshotData, error) {
	var data db.SnapshotData
	if len(nf.Pods) == 0 {
		return data, fmt.Errorf("the network needs at least one pod")
	}
	names := map[string]bool{}
	for _, p := range nf.Pods {
		if strings.TrimSpace(p.Name) == "" {
			return data, fmt.Errorf("every pod needs a name")
		}
		names[p.Name] = true
		data.Pods = append(data.Pods, db.PodRow{Name: p.Name, Location: p.Location, Pairing: p.Pairing, DevCount: p.DevCount, Streams: p.Streams})
		st := nf.Stats[p.Name] // zero value when unspecified → defaults below
		p50, p85 := st.CycleP50, st.CycleP85
		if p50 <= 0 {
			p50 = 7
		}
		if p85 <= 0 {
			p85 = p50 * 2.4
		}
		wip := st.Wip
		if wip <= 0 {
			wip = float64(p.DevCount)
		}
		tp := st.ThroughputPerWeek
		if tp <= 0 {
			tp = 2
		}
		data.Stats = append(data.Stats, db.PodStatRow{
			Pod: p.Name, P50: p50, P85: p85, Mean: (p50 + p85) / 2,
			Mu: math.Log(p50), Sigma: 0.6, WipCount: int(wip + 0.5), ThroughputPerWk: tp,
		})
		if st.Hygiene > 0 {
			sc := st.Hygiene
			data.Hygiene = append(data.Hygiene, db.PodHygieneRow{Pod: p.Name, Score: &sc})
		}
	}
	for _, e := range nf.Edges { // keep only edges between known pods
		if names[e.From] && names[e.To] && e.From != e.To {
			data.Edges = append(data.Edges, db.EdgeRow{From: e.From, To: e.To, Count: e.Count})
		}
	}
	return data, nil
}

// visibleSnapshot loads a snapshot the caller may read (owner, admin, public, or
// baseline). Returns nil after writing an error response.
func (s *server) visibleSnapshot(w http.ResponseWriter, id string, c auth.Claims) *db.SnapshotRow {
	row, err := s.db.GetSnapshot(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return nil
	}
	if row == nil {
		http.Error(w, "snapshot not found", 404)
		return nil
	}
	if row.Owner != c.Sub && !c.Has("admin") && !row.Public && row.Source != "baseline" {
		http.Error(w, "forbidden", 403)
		return nil
	}
	return row
}

// handleSnapshotExport (GET /api/snapshots/{id}/export) downloads the editable
// NetworkFile for any snapshot the caller can see.
func (s *server) handleSnapshotExport(w http.ResponseWriter, id string, c auth.Claims) {
	row := s.visibleSnapshot(w, id, c)
	if row == nil {
		return
	}
	nf, err := s.snapshotToNetwork(id, row.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	body, _ := json.MarshalIndent(nf, "", "  ")
	fname := safeFilename(row.Name) + ".network.json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`"`)
	w.Write(body)
}

// snapshotFromNetwork persists a NetworkFile as a new template snapshot.
func (s *server) snapshotFromNetwork(nf NetworkFile, owner, name string) (string, error) {
	data, err := networkToData(nf)
	if err != nil {
		return "", err
	}
	id := newID()
	if err := s.db.CreateSnapshotWithData(db.SnapshotRow{
		ID: id, Owner: owner, Name: name, Source: "template", CreatedAt: time.Now().Unix(),
	}, data); err != nil {
		return "", err
	}
	return id, nil
}

// handleNetworkImport (POST /api/snapshots/import-network) creates a template
// from an uploaded NetworkFile (multipart or raw JSON).
func (s *server) handleNetworkImport(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "templates require the database", 503)
		return
	}
	data, err := readUpload(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var nf NetworkFile
	if err := json.Unmarshal(data, &nf); err != nil {
		http.Error(w, "not a valid network file: "+err.Error(), 400)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = strings.TrimSpace(nf.Name)
	}
	if name == "" {
		name = "Imported scenario"
	}
	id, err := s.snapshotFromNetwork(nf, c.Sub, name)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"id": id, "name": name, "pods": len(nf.Pods), "edges": len(nf.Edges)})
}

// handleSnapshotClone (POST /api/snapshots/{id}/clone) copies a visible snapshot
// into a new editable template owned by the caller.
func (s *server) handleSnapshotClone(w http.ResponseWriter, r *http.Request, id string, c auth.Claims) {
	row := s.visibleSnapshot(w, id, c)
	if row == nil {
		return
	}
	var b struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&b)
	name := strings.TrimSpace(b.Name)
	if name == "" {
		name = row.Name + " (copy)"
	}
	nf, err := s.snapshotToNetwork(id, name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	newID, err := s.snapshotFromNetwork(*nf, c.Sub, name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"id": newID, "name": name})
}

// handleSampleNetwork (GET /api/sample/network.json) serves a small worked
// example of the editable format.
func (s *server) handleSampleNetwork(w http.ResponseWriter, r *http.Request) {
	nf := NetworkFile{
		Name: "Sample scenario",
		Pods: []NetPod{
			{Name: "Platform", Location: "San Mateo", Pairing: true, DevCount: 6},
			{Name: "Payments", Location: "Bengaluru", Pairing: true, DevCount: 5},
			{Name: "Mobile", Location: "Toronto", Pairing: false, DevCount: 4},
		},
		Edges: []NetEdge{{From: "Payments", To: "Platform", Count: 5}, {From: "Mobile", To: "Platform", Count: 3}},
		Stats: map[string]NetStat{
			"Platform": {Wip: 14, ThroughputPerWeek: 3, CycleP50: 8, CycleP85: 20, Hygiene: 0.6},
			"Payments": {Wip: 9, ThroughputPerWeek: 2.5, CycleP50: 6, CycleP85: 15, Hygiene: 0.5},
			"Mobile":   {Wip: 6, ThroughputPerWeek: 2, CycleP50: 5, CycleP85: 12, Hygiene: 0.7},
		},
	}
	body, _ := json.MarshalIndent(nf, "", "  ")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="conway-sample.network.json"`)
	w.Write(body)
}

// handleSampleRoster (GET /api/sample/roster.csv) serves a correctly-shaped
// team roster: headcount + pairing (engine halves pairs into work-streams), or
// an explicit Work-streams column for teams that prefer to state pair count.
func (s *server) handleSampleRoster(w http.ResponseWriter, r *http.Request) {
	csv := "Pod Name,Developers,Pairs,Location,Work-streams\n" +
		"Platform,6,yes,San Mateo,\n" +
		"Payments,5,yes,Bengaluru,\n" +
		"Mobile,4,no,Toronto,\n" +
		"SRE,8,yes,Kraków,3\n"
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="conway-roster-sample.csv"`)
	w.Write([]byte(csv))
}

// safeFilename reduces a name to a download-safe slug.
func safeFilename(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == ' ' || r == '-' || r == '_':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "network"
	}
	return string(out)
}
