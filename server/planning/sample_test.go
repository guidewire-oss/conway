package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"slices"
)

// specs/033-consistent-plan-controls-and-samples.md:42
var _ = Describe("roster-aware initiative samples", func() {
	It("round trips example work using every supplied team and no demo names", func() {
		teams := []Team{{Name: "Atlas & Services", Tracks: 2}, {Name: "Beacon", Tracks: 1}, {Name: "Cedar", Tracks: 3}}
		rows, err := ReadGrid(WriteSampleInitiativesXLSX(teams), "")
		Expect(err).NotTo(HaveOccurred())
		names := []string{"Atlas & Services", "Beacon", "Cedar"}
		start := slices.Index(rows[0], "Atlas & Services Sequence")
		Expect(start).To(BeNumerically(">=", 0), "sample must include the first roster team's sequence column")
		Expect(rows[0][start:]).To(Equal([]string{"Atlas & Services Sequence", "Atlas & Services", "Beacon Sequence", "Beacon", "Cedar Sequence", "Cedar"}))
		parsed := ParseMatrix(rows, names, true)
		Expect(parsed.Initiatives).NotTo(BeEmpty())
		for _, name := range names {
			Expect(parsed.Initiatives[0].Work).To(HaveKey(name))
			Expect(parsed.Initiatives[0].Work[name].InPath).To(BeTrue())
		}
		Expect(parsed.Initiatives[0].Work).To(HaveLen(len(teams)))
	})
	It("keeps the existing demo sample when no teams are attached", func() {
		teams, inits := Demo()
		Expect(WriteSampleInitiativesXLSX(nil)).To(Equal(WriteInitiativesXLSX(teams, inits)))
	})
	It("produces an importable sample for a single team", func() {
		rows, err := ReadGrid(WriteSampleInitiativesXLSX([]Team{{Name: "Atlas", Tracks: 1}}), "")
		Expect(err).NotTo(HaveOccurred())
		parsed := ParseMatrix(rows, []string{"Atlas"}, true)
		Expect(parsed.Initiatives).To(HaveLen(1))
		Expect(parsed.Initiatives[0].Work["Atlas"].Weeks).To(Equal(2.0))
	})
})
