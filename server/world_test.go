package main

import (
	"testing"
)

// buildWorld is the one parser for a snapshot's dynamic docs (see tableDoc in
// snapshotdocs.go) — this is a basic sanity check that it parses a doc set
// (here, the testdata/ fixture) into a fully-populated World.
func TestBuildWorldFromDocs(t *testing.T) {
	w := testWorld(t)

	if len(w.Pods) == 0 {
		t.Fatal("expected at least one pod")
	}
	if len(w.Edges) == 0 {
		t.Fatal("expected at least one edge")
	}
	for _, p := range w.Pods {
		s, ok := w.Stats[p.Name]
		if !ok {
			t.Fatalf("pod %s has no stats", p.Name)
		}
		if s.ThroughputWk <= 0 {
			t.Fatalf("pod %s: throughput should default to a positive value, got %v", p.Name, s.ThroughputWk)
		}
	}
}
