package main

import (
	"encoding/json"
	"testing"

	"conway/db"
)

// A NetworkFile converts to snapshot table rows: pods, edges (between known
// pods), and per-pod stats (with defaults) + hygiene.
func TestNetworkToDataRows(t *testing.T) {
	nf := NetworkFile{
		Name: "T",
		Pods: []NetPod{
			{Name: "A", Location: "X", Pairing: true, DevCount: 6},
			{Name: "B", Location: "X", Pairing: false, DevCount: 4},
			{Name: "C", Location: "Y", Pairing: true, DevCount: 2},
		},
		Edges: []NetEdge{{From: "A", To: "B", Count: 3}, {From: "C", To: "A", Count: 1}, {From: "A", To: "Z", Count: 9}},
		Stats: map[string]NetStat{"A": {Wip: 12, ThroughputPerWeek: 3, CycleP50: 8, CycleP85: 20, Hygiene: 0.6}},
	}
	data, err := networkToData(nf)
	if err != nil {
		t.Fatalf("networkToData: %v", err)
	}
	if len(data.Pods) != 3 {
		t.Fatalf("pods = %d, want 3", len(data.Pods))
	}
	if len(data.Edges) != 2 { // A→Z dropped (Z unknown)
		t.Fatalf("edges = %d, want 2 (A→Z dropped)", len(data.Edges))
	}
	var a *db.PodStatRow
	for i := range data.Stats {
		if data.Stats[i].Pod == "A" {
			a = &data.Stats[i]
		}
	}
	if a == nil || a.WipCount != 12 || a.P50 != 8 || a.P85 != 20 {
		t.Fatalf("A stat wrong: %+v", a)
	}
	if len(data.Hygiene) != 1 || data.Hygiene[0].Pod != "A" {
		t.Fatalf("hygiene rows wrong: %+v", data.Hygiene)
	}
}

func TestNetworkToDataRejectsEmpty(t *testing.T) {
	if _, err := networkToData(NetworkFile{}); err == nil {
		t.Fatal("empty network should error")
	}
}

// pods.json (generated from table rows) derives overlap from site: same-site
// pods coordinate cheaply (6h), cross-site is the slow seam (2h).
func TestPodsDocOverlap(t *testing.T) {
	doc := podsDocFromRows([]db.PodRow{{Name: "A", Location: "X"}, {Name: "B", Location: "X"}, {Name: "C", Location: "Y"}})
	var out struct {
		Pods    []map[string]any              `json:"pods"`
		Overlap map[string]map[string]float64 `json:"overlap"`
	}
	if err := json.Unmarshal(doc, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Pods) != 3 {
		t.Fatalf("pods = %d", len(out.Pods))
	}
	if out.Overlap["A"]["B"] != 6 || out.Overlap["A"]["C"] != 2 || out.Overlap["A"]["A"] != 8 {
		t.Fatalf("overlap wrong: A-B=%v A-C=%v A-A=%v", out.Overlap["A"]["B"], out.Overlap["A"]["C"], out.Overlap["A"]["A"])
	}
}

func TestSafeFilename(t *testing.T) {
	if got := safeFilename("Q3 2026: Crisis!"); got != "q3-2026-crisis" {
		t.Fatalf("safeFilename = %q", got)
	}
}
