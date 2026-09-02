package main

import (
	"encoding/json"

	"conway/server/db"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A NetworkFile converts to snapshot table rows: pods, edges (between known
// pods), and per-pod stats (with defaults) + hygiene.
var _ = Describe("network file conversion", func() {
	It("converts to pods, known-pod edges, stats and hygiene rows", func() {
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
		Expect(err).NotTo(HaveOccurred())
		Expect(data.Pods).To(HaveLen(3))
		Expect(data.Edges).To(HaveLen(2), "A→Z dropped (Z unknown)")
		var a *db.PodStatRow
		for i := range data.Stats {
			if data.Stats[i].Pod == "A" {
				a = &data.Stats[i]
			}
		}
		Expect(a).NotTo(BeNil())
		Expect(a.WipCount).To(Equal(12))
		Expect(a.P50).To(Equal(8.0))
		Expect(a.P85).To(Equal(20.0))
		Expect(data.Hygiene).To(HaveLen(1))
		Expect(data.Hygiene[0].Pod).To(Equal("A"))
	})

	It("rejects an empty network", func() {
		_, err := networkToData(NetworkFile{})
		Expect(err).To(HaveOccurred(), "empty network should error")
	})
})

// pods.json (generated from table rows) derives overlap from site: same-site
// pods coordinate cheaply (6h), cross-site is the slow seam (2h).
var _ = Describe("the pods doc", func() {
	It("derives overlap from site strings", func() {
		doc := podsDocFromRows([]db.PodRow{{Name: "A", Location: "X"}, {Name: "B", Location: "X"}, {Name: "C", Location: "Y"}})
		var out struct {
			Pods    []map[string]any              `json:"pods"`
			Overlap map[string]map[string]float64 `json:"overlap"`
		}
		Expect(json.Unmarshal(doc, &out)).NotTo(HaveOccurred())
		Expect(out.Pods).To(HaveLen(3))
		Expect(out.Overlap["A"]["B"]).To(Equal(6.0), "same site")
		Expect(out.Overlap["A"]["C"]).To(Equal(2.0), "cross site")
		Expect(out.Overlap["A"]["A"]).To(Equal(8.0), "self")
	})
})

var _ = Describe("safeFilename", func() {
	It("slugifies human names", func() {
		Expect(safeFilename("Q3 2026: Crisis!")).To(Equal("q3-2026-crisis"))
	})
})
