package planning

import (
	"strings"
	"testing"
)

// v2 layout: paired "<Team> Sequence" (deps) + "<Team>" (weeks) columns.
func sampleRows() [][]string {
	return [][]string{
		{"S. No", "Initiative ", "PM Lead", "Engg Lead", "Architect lead", "PgM Lead",
			"Estimate in Weeks across teams required for Full Kit",
			"Ajanta Sequence", "Ajanta", " Bandipur Sequence", "Bandipur", "Cooperstown Sequence", "Cooperstown"},
		// Ajanta waits on Bandipur+Cooperstown (note the messy whitespace); Bandipur NONE; Cooperstown unfilled placeholder/TBD
		{"1", "Billing revamp\nThe description", "Alice", "Bob", "", "",
			"0", "Bandipur,  Cooperstown ", "3", "NONE", "2",
			"replace this with a pod name, who needs to do some work", "TBD"},
		// second initiative: Cooperstown waits on Bandipur
		{"2", "Claims UI", "", "", "", "",
			"0", "NONE", "4", "", "", "Bandipur", "5"},
	}
}

func TestParseMatrixV2(t *testing.T) {
	p := ParseMatrix(sampleRows(), nil, false)
	if got := p.Teams; len(got) != 3 || got[0] != "Ajanta" || got[1] != "Bandipur" || got[2] != "Cooperstown" {
		t.Fatalf("teams: %v", got)
	}
	if len(p.Initiatives) != 2 {
		t.Fatalf("want 2 initiatives, got %d", len(p.Initiatives))
	}
	a := p.Initiatives[0]
	if a.Name != "Billing revamp" || a.Description != "The description" {
		t.Fatalf("name/desc: %q / %q", a.Name, a.Description)
	}
	if a.Leads["pm"] != "Alice" || a.Leads["eng"] != "Bob" {
		t.Fatalf("leads: %v", a.Leads)
	}
	aj := a.Work["Ajanta"]
	if aj.Weeks != 3 || !aj.Estimated || !aj.InPath {
		t.Fatalf("Ajanta work: %+v", aj)
	}
	if len(aj.DependsOn) != 2 || aj.DependsOn[0] != "Bandipur" || aj.DependsOn[1] != "Cooperstown" {
		t.Fatalf("Ajanta deps (whitespace must be trimmed): %v", aj.DependsOn)
	}
	if bp := a.Work["Bandipur"]; bp.Weeks != 2 || len(bp.DependsOn) != 0 {
		t.Fatalf("Bandipur (NONE -> no deps): %+v", bp)
	}
	if _, ok := a.Work["Cooperstown"]; ok {
		t.Fatalf("Cooperstown was TBD + placeholder — should not be recorded for initiative 1")
	}
}

// Some real-world FullKit sheets pair columns as "<Team> Dependencies" + "<Team>"
// instead of "<Team> Sequence" + "<Team>" — both must parse identically.
func sampleRowsDependenciesHeader() [][]string {
	return [][]string{
		{"S. No", "Initiative ", "PM Lead", "Engg Lead", "Architect lead", "PgM Lead",
			"Estimate in Weeks across teams required for Full Kit",
			"Ajanta Dependencies", "Ajanta", " Bandipur Dependencies", "Bandipur", "Cooperstown Dependencies", "Cooperstown"},
		{"1", "Billing revamp\nThe description", "Alice", "Bob", "", "",
			"0", "Bandipur,  Cooperstown ", "3", "NONE", "2",
			"replace this with a pod name, who needs to do some work", "TBD"},
		{"2", "Claims UI", "", "", "", "",
			"0", "NONE", "4", "", "", "Bandipur", "5"},
	}
}

func TestParseMatrixDependenciesHeaderVariant(t *testing.T) {
	p := ParseMatrix(sampleRowsDependenciesHeader(), nil, false)
	if got := p.Teams; len(got) != 3 || got[0] != "Ajanta" || got[1] != "Bandipur" || got[2] != "Cooperstown" {
		t.Fatalf("teams: %v", got)
	}
	a := p.Initiatives[0]
	aj := a.Work["Ajanta"]
	if aj.Weeks != 3 || !aj.Estimated || !aj.InPath {
		t.Fatalf("Ajanta work: %+v", aj)
	}
	if len(aj.DependsOn) != 2 || aj.DependsOn[0] != "Bandipur" || aj.DependsOn[1] != "Cooperstown" {
		t.Fatalf("Ajanta deps (Dependencies-suffix header must parse like Sequence): %v", aj.DependsOn)
	}
	net := BuildNetwork(p)
	if len(net.Edges) == 0 {
		t.Fatal("Dependencies-suffix header must still produce network edges")
	}
}

// A dependency cell may be free text ("Requirements unknown", a PM's aside)
// rather than an actual pod name — strict mode drops anything that doesn't
// case-insensitively match a roster pod, so it can't become a phantom
// network node.
func sampleRowsMessyDeps() [][]string {
	return [][]string{
		{"Initiative ", "Estimate in Weeks across teams required for Full Kit",
			"Ajanta Dependencies", "Ajanta", "Bandipur Dependencies", "Bandipur"},
		{"Billing revamp", "0",
			"  bandipur ,Requirements unknown, COOPERSTOWN  ", "3", "NONE", "2"},
	}
}

func TestParseMatrixStrictDropsUnknownDeps(t *testing.T) {
	roster := []string{"Ajanta", "Bandipur", "Cooperstown"}
	p := ParseMatrix(sampleRowsMessyDeps(), roster, true)
	aj := p.Initiatives[0].Work["Ajanta"]
	if len(aj.DependsOn) != 2 {
		t.Fatalf("strict deps = %v, want 2 roster matches (bandipur/COOPERSTOWN), Requirements-unknown dropped", aj.DependsOn)
	}
	for _, d := range aj.DependsOn {
		if !strings.EqualFold(d, "bandipur") && !strings.EqualFold(d, "cooperstown") {
			t.Fatalf("unexpected dep survived strict filtering: %q", d)
		}
	}
}

func TestParseMatrixStrictIsCaseInsensitiveAndTrimmed(t *testing.T) {
	roster := []string{"Ajanta", "Bandipur", "Cooperstown"}
	p := ParseMatrix(sampleRowsMessyDeps(), roster, true)
	aj := p.Initiatives[0].Work["Ajanta"]
	found := map[string]bool{}
	for _, d := range aj.DependsOn {
		found[strings.ToLower(strings.TrimSpace(d))] = true
	}
	if !found["bandipur"] || !found["cooperstown"] {
		t.Fatalf("case/whitespace variants must still match roster names: %v", aj.DependsOn)
	}
}

func TestParseMatrixStrictWithoutRosterKeepsDepsUnfiltered(t *testing.T) {
	// strict=true but no roster given (e.g. initiatives uploaded before a
	// roster is attached) — nothing to validate against, so don't silently
	// wipe every dependency.
	p := ParseMatrix(sampleRowsMessyDeps(), nil, true)
	aj := p.Initiatives[0].Work["Ajanta"]
	if len(aj.DependsOn) != 3 {
		t.Fatalf("with no roster, strict mode should not filter: %v", aj.DependsOn)
	}
}

func TestParseMatrixNonStrictKeepsFreeText(t *testing.T) {
	roster := []string{"Ajanta", "Bandipur", "Cooperstown"}
	p := ParseMatrix(sampleRowsMessyDeps(), roster, false)
	aj := p.Initiatives[0].Work["Ajanta"]
	if len(aj.DependsOn) != 3 {
		t.Fatalf("non-strict mode must keep every parsed dep including free text: %v", aj.DependsOn)
	}
}

func TestParseMatrixSupportsMultipleCommaSeparatedDeps(t *testing.T) {
	p := ParseMatrix(sampleRowsMessyDeps(), nil, false)
	aj := p.Initiatives[0].Work["Ajanta"]
	if len(aj.DependsOn) != 3 || aj.DependsOn[0] != "bandipur" || aj.DependsOn[2] != "COOPERSTOWN" {
		t.Fatalf("comma-separated deps not split/trimmed correctly: %v", aj.DependsOn)
	}
}

func TestBuildNetworkDirected(t *testing.T) {
	net := BuildNetwork(ParseMatrix(sampleRows(), nil, false))
	edge := func(from, to string) (Edge, bool) {
		for _, e := range net.Edges {
			if e.From == from && e.To == to {
				return e, true
			}
		}
		return Edge{}, false
	}
	if _, ok := edge("Bandipur", "Ajanta"); !ok {
		t.Fatal("expected directed edge Bandipur -> Ajanta")
	}
	if _, ok := edge("Cooperstown", "Ajanta"); !ok {
		t.Fatal("expected directed edge Cooperstown -> Ajanta")
	}
	if _, ok := edge("Bandipur", "Cooperstown"); !ok {
		t.Fatal("expected directed edge Bandipur -> Cooperstown (initiative 2)")
	}
	node := func(team string) Node {
		for _, n := range net.Nodes {
			if n.Team == team {
				return n
			}
		}
		t.Fatalf("no node %s", team)
		return Node{}
	}
	if n := node("Ajanta"); n.WaitsOn != 2 {
		t.Fatalf("Ajanta should wait on 2: %+v", n)
	}
	if n := node("Bandipur"); n.Blocks != 2 { // blocks Ajanta + Cooperstown
		t.Fatalf("Bandipur should block 2: %+v", n)
	}
	// load: Ajanta 3+4=7, Cooperstown 5, Bandipur 2 -> Ajanta is the heaviest node
	if net.Nodes[0].Team != "Ajanta" {
		t.Fatalf("heaviest node should be Ajanta, got %s", net.Nodes[0].Team)
	}
}
