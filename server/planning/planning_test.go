package planning

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("ParseMatrix", func() {
	It("parses the v2 paired layout end to end", func() {
		p := ParseMatrix(sampleRows(), nil, false)
		Expect(p.Teams).To(Equal([]string{"Alpha", "Beta", "Gamma"}))
		Expect(p.Initiatives).To(HaveLen(2))
		a := p.Initiatives[0]
		Expect(a.Name).To(Equal("Billing revamp"))
		Expect(a.Description).To(Equal("The description"))
		Expect(a.Leads["pm"]).To(Equal("Alice"))
		Expect(a.Leads["eng"]).To(Equal("Bob"))
		aj := a.Work["Alpha"]
		Expect(aj.Weeks).To(Equal(3.0))
		Expect(aj.Estimated).To(BeTrue())
		Expect(aj.InPath).To(BeTrue())
		Expect(aj.DependsOn).To(Equal([]string{"Beta", "Gamma"}), "whitespace trimmed")
		bp := a.Work["Beta"]
		Expect(bp.Weeks).To(Equal(2.0))
		Expect(bp.DependsOn).To(BeEmpty(), "NONE -> no deps")
		_, ok := a.Work["Gamma"]
		Expect(ok).To(BeFalse(), "TBD + placeholder column must not be recorded")
	})

	It("parses the Dependencies-suffix header variant identically", func() {
		p := ParseMatrix(sampleRowsDependenciesHeader(), nil, false)
		Expect(p.Teams).To(Equal([]string{"Alpha", "Beta", "Gamma"}))
		aj := p.Initiatives[0].Work["Alpha"]
		Expect(aj.Weeks).To(Equal(3.0))
		Expect(aj.Estimated).To(BeTrue())
		Expect(aj.DependsOn).To(Equal([]string{"Beta", "Gamma"}),
			"the Dependencies-suffix header must parse like Sequence")
		net := BuildNetwork(p)
		Expect(net.Edges).NotTo(BeEmpty())
	})

	It("strict mode drops deps that match no roster pod", func() {
		roster := []string{"Alpha", "Beta", "Gamma"}
		p := ParseMatrix(sampleRowsMessyDeps(), roster, true)
		aj := p.Initiatives[0].Work["Alpha"]
		Expect(aj.DependsOn).To(HaveLen(2), "beta/GAMMA survive, Requirements-unknown drops")
		for _, d := range aj.DependsOn {
			Expect(strings.EqualFold(d, "beta") || strings.EqualFold(d, "gamma")).To(BeTrue(),
				"unexpected dep survived strict filtering: %q", d)
		}
	})

	It("strict mode matches case-insensitively and trimmed", func() {
		roster := []string{"Alpha", "Beta", "Gamma"}
		p := ParseMatrix(sampleRowsMessyDeps(), roster, true)
		aj := p.Initiatives[0].Work["Alpha"]
		found := map[string]bool{}
		for _, d := range aj.DependsOn {
			found[strings.ToLower(strings.TrimSpace(d))] = true
		}
		Expect(found).To(HaveKey("beta"))
		Expect(found).To(HaveKey("gamma"))
	})

	It("keeps deps unfiltered in strict mode when no roster is given", func() {
		// strict=true but no roster (initiatives uploaded before a roster is
		// attached) — nothing to validate against, so don't silently wipe
		// every dependency.
		p := ParseMatrix(sampleRowsMessyDeps(), nil, true)
		Expect(p.Initiatives[0].Work["Alpha"].DependsOn).To(HaveLen(3),
			"with no roster, strict mode must not filter")
	})

	It("keeps free-text deps in non-strict mode", func() {
		roster := []string{"Alpha", "Beta", "Gamma"}
		p := ParseMatrix(sampleRowsMessyDeps(), roster, false)
		Expect(p.Initiatives[0].Work["Alpha"].DependsOn).To(HaveLen(3),
			"non-strict mode must keep every parsed dep including free text")
	})

	It("splits and trims comma-separated deps", func() {
		p := ParseMatrix(sampleRowsMessyDeps(), nil, false)
		aj := p.Initiatives[0].Work["Alpha"]
		Expect(aj.DependsOn).To(Equal([]string{"beta", "Requirements unknown", "GAMMA"}))
	})
})

var _ = Describe("BuildNetwork", func() {
	It("derives directed edges and per-node waits/blocks counts", func() {
		net := BuildNetwork(ParseMatrix(sampleRows(), nil, false))
		edge := func(from, to string) (Edge, bool) {
			for _, e := range net.Edges {
				if e.From == from && e.To == to {
					return e, true
				}
			}
			return Edge{}, false
		}
		_, ok := edge("Beta", "Alpha")
		Expect(ok).To(BeTrue(), "expected directed edge Beta -> Alpha")
		_, ok = edge("Gamma", "Alpha")
		Expect(ok).To(BeTrue(), "expected directed edge Gamma -> Alpha")
		_, ok = edge("Beta", "Gamma")
		Expect(ok).To(BeTrue(), "expected directed edge Beta -> Gamma (initiative 2)")
		node := func(team string) Node {
			for _, n := range net.Nodes {
				if n.Team == team {
					return n
				}
			}
			Fail("no node " + team)
			return Node{}
		}
		Expect(node("Alpha").WaitsOn).To(Equal(2))
		Expect(node("Beta").Blocks).To(Equal(2), "Beta blocks Alpha + Gamma")
		// load: Alpha 3+4=7, Gamma 5, Beta 2 -> Alpha is the heaviest node
		Expect(net.Nodes[0].Team).To(Equal("Alpha"), "heaviest node first")
	})
})
