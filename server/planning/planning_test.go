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
			"Alpha Sequence", "Alpha", " Beta Sequence", "Beta", "Gamma Sequence", "Gamma"},
		// Alpha waits on Beta+Gamma (note the messy whitespace); Beta NONE; Gamma unfilled placeholder/TBD
		{"1", "Billing revamp\nThe description", "Alice", "Bob", "", "",
			"0", "Beta,  Gamma ", "3", "NONE", "2",
			"replace this with a pod name, who needs to do some work", "TBD"},
		// second initiative: Gamma waits on Beta
		{"2", "Claims UI", "", "", "", "",
			"0", "NONE", "4", "", "", "Beta", "5"},
	}
}

func TestParseMatrixV2(t *testing.T) {
	p := ParseMatrix(sampleRows(), nil, false)
	if got := p.Teams; len(got) != 3 || got[0] != "Alpha" || got[1] != "Beta" || got[2] != "Gamma" {
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
	aj := a.Work["Alpha"]
	if aj.Weeks != 3 || !aj.Estimated || !aj.InPath {
		t.Fatalf("Alpha work: %+v", aj)
	}
	if len(aj.DependsOn) != 2 || aj.DependsOn[0] != "Beta" || aj.DependsOn[1] != "Gamma" {
		t.Fatalf("Alpha deps (whitespace must be trimmed): %v", aj.DependsOn)
	}
	if bp := a.Work["Beta"]; bp.Weeks != 2 || len(bp.DependsOn) != 0 {
		t.Fatalf("Beta (NONE -> no deps): %+v", bp)
	}
	if _, ok := a.Work["Gamma"]; ok {
		t.Fatalf("Gamma was TBD + placeholder — should not be recorded for initiative 1")
	}
}

// Some real-world FullKit sheets pair columns as "<Team> Dependencies" + "<Team>"
// instead of "<Team> Sequence" + "<Team>" — both must parse identically.
func sampleRowsDependenciesHeader() [][]string {
	return [][]string{
		{"S. No", "Initiative ", "PM Lead", "Engg Lead", "Architect lead", "PgM Lead",
			"Estimate in Weeks across teams required for Full Kit",
			"Alpha Dependencies", "Alpha", " Beta Dependencies", "Beta", "Gamma Dependencies", "Gamma"},
		{"1", "Billing revamp\nThe description", "Alice", "Bob", "", "",
			"0", "Beta,  Gamma ", "3", "NONE", "2",
			"replace this with a pod name, who needs to do some work", "TBD"},
		{"2", "Claims UI", "", "", "", "",
			"0", "NONE", "4", "", "", "Beta", "5"},
	}
}

func TestParseMatrixDependenciesHeaderVariant(t *testing.T) {
	p := ParseMatrix(sampleRowsDependenciesHeader(), nil, false)
	if got := p.Teams; len(got) != 3 || got[0] != "Alpha" || got[1] != "Beta" || got[2] != "Gamma" {
		t.Fatalf("teams: %v", got)
	}
	a := p.Initiatives[0]
	aj := a.Work["Alpha"]
	if aj.Weeks != 3 || !aj.Estimated || !aj.InPath {
		t.Fatalf("Alpha work: %+v", aj)
	}
	if len(aj.DependsOn) != 2 || aj.DependsOn[0] != "Beta" || aj.DependsOn[1] != "Gamma" {
		t.Fatalf("Alpha deps (Dependencies-suffix header must parse like Sequence): %v", aj.DependsOn)
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
			"Alpha Dependencies", "Alpha", "Beta Dependencies", "Beta"},
		{"Billing revamp", "0",
			"  beta ,Requirements unknown, GAMMA  ", "3", "NONE", "2"},
	}
}

func TestParseMatrixStrictDropsUnknownDeps(t *testing.T) {
	roster := []string{"Alpha", "Beta", "Gamma"}
	p := ParseMatrix(sampleRowsMessyDeps(), roster, true)
	aj := p.Initiatives[0].Work["Alpha"]
	if len(aj.DependsOn) != 2 {
		t.Fatalf("strict deps = %v, want 2 roster matches (beta/GAMMA), Requirements-unknown dropped", aj.DependsOn)
	}
	for _, d := range aj.DependsOn {
		if !strings.EqualFold(d, "beta") && !strings.EqualFold(d, "gamma") {
			t.Fatalf("unexpected dep survived strict filtering: %q", d)
		}
	}
}

func TestParseMatrixStrictIsCaseInsensitiveAndTrimmed(t *testing.T) {
	roster := []string{"Alpha", "Beta", "Gamma"}
	p := ParseMatrix(sampleRowsMessyDeps(), roster, true)
	aj := p.Initiatives[0].Work["Alpha"]
	found := map[string]bool{}
	for _, d := range aj.DependsOn {
		found[strings.ToLower(strings.TrimSpace(d))] = true
	}
	if !found["beta"] || !found["gamma"] {
		t.Fatalf("case/whitespace variants must still match roster names: %v", aj.DependsOn)
	}
}

func TestParseMatrixStrictWithoutRosterKeepsDepsUnfiltered(t *testing.T) {
	// strict=true but no roster given (e.g. initiatives uploaded before a
	// roster is attached) — nothing to validate against, so don't silently
	// wipe every dependency.
	p := ParseMatrix(sampleRowsMessyDeps(), nil, true)
	aj := p.Initiatives[0].Work["Alpha"]
	if len(aj.DependsOn) != 3 {
		t.Fatalf("with no roster, strict mode should not filter: %v", aj.DependsOn)
	}
}

func TestParseMatrixNonStrictKeepsFreeText(t *testing.T) {
	roster := []string{"Alpha", "Beta", "Gamma"}
	p := ParseMatrix(sampleRowsMessyDeps(), roster, false)
	aj := p.Initiatives[0].Work["Alpha"]
	if len(aj.DependsOn) != 3 {
		t.Fatalf("non-strict mode must keep every parsed dep including free text: %v", aj.DependsOn)
	}
}

func TestParseMatrixSupportsMultipleCommaSeparatedDeps(t *testing.T) {
	p := ParseMatrix(sampleRowsMessyDeps(), nil, false)
	aj := p.Initiatives[0].Work["Alpha"]
	if len(aj.DependsOn) != 3 || aj.DependsOn[0] != "beta" || aj.DependsOn[2] != "GAMMA" {
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
	if _, ok := edge("Beta", "Alpha"); !ok {
		t.Fatal("expected directed edge Beta -> Alpha")
	}
	if _, ok := edge("Gamma", "Alpha"); !ok {
		t.Fatal("expected directed edge Gamma -> Alpha")
	}
	if _, ok := edge("Beta", "Gamma"); !ok {
		t.Fatal("expected directed edge Beta -> Gamma (initiative 2)")
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
	if n := node("Alpha"); n.WaitsOn != 2 {
		t.Fatalf("Alpha should wait on 2: %+v", n)
	}
	if n := node("Beta"); n.Blocks != 2 { // blocks Alpha + Gamma
		t.Fatalf("Beta should block 2: %+v", n)
	}
	// load: Alpha 3+4=7, Gamma 5, Beta 2 -> Alpha is the heaviest node
	if net.Nodes[0].Team != "Alpha" {
		t.Fatalf("heaviest node should be Alpha, got %s", net.Nodes[0].Team)
	}
}
