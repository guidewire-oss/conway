package main

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// buildWorld is the one parser for a snapshot's dynamic docs (see tableDoc in
// snapshotdocs.go) — a sanity check that it parses the testdata/ fixture into
// a fully-populated World.
var _ = Describe("buildWorld", func() {
	It("parses a doc set into a fully-populated World", func() {
		w := testWorld()
		Expect(w.Pods).NotTo(BeEmpty())
		Expect(w.Edges).NotTo(BeEmpty())
		for _, p := range w.Pods {
			s, ok := w.Stats[p.Name]
			Expect(ok).To(BeTrue(), "pod %s has no stats", p.Name)
			Expect(s.ThroughputWk).To(BeNumerically(">", 0),
				"pod %s: throughput should default to a positive value", p.Name)
		}
	})
})
