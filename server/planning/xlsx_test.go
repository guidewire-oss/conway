package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// buildXLSX reuses the production writer so tests exercise the same code path.
func buildXLSX(rows [][]string) []byte { return WriteXLSX("FullKit exercise", rows) }

func readCSV(b []byte) [][]string {
	rows, err := ReadGrid(b, "")
	Expect(err).NotTo(HaveOccurred())
	return rows
}

var _ = Describe("the XLSX round-trip", func() {
	It("writes and reads the initiatives matrix without losing cells", func() {
		rows, err := ReadXLSX(buildXLSX(sampleRows()), "FullKit exercise")
		Expect(err).NotTo(HaveOccurred())
		got := ParseMatrix(rows, nil, false)
		Expect(got.Teams).To(HaveLen(3))
		Expect(got.Initiatives).To(HaveLen(2))
		aj := got.Initiatives[0].Work["Alpha"]
		Expect(aj.Weeks).To(Equal(3.0))
		Expect(aj.Estimated).To(BeTrue(), "numeric cell lost in round-trip")
		Expect(aj.DependsOn).To(Equal([]string{"Beta", "Gamma"}), "deps lost/untrimmed in round-trip")
	})

	It("matches the sheet name case-insensitively and trimmed", func() {
		_, err := ReadXLSX(buildXLSX(sampleRows()), "  fullkit EXERCISE ")
		Expect(err).NotTo(HaveOccurred())
	})

	It("round-trips the generated sample initiatives and teams CSV", func() {
		teams, inits := Demo()
		rows, err := ReadXLSX(WriteInitiativesXLSX(teams, inits), "FullKit exercise")
		Expect(err).NotTo(HaveOccurred())
		got := ParseMatrix(rows, nil, false)
		Expect(got.Initiatives).To(HaveLen(len(inits)))
		teamsBack, err := ParseTeamsRows(readCSV(WriteTeamsCSV(teams)))
		Expect(err).NotTo(HaveOccurred())
		Expect(teamsBack).To(HaveLen(len(teams)))
		// a known dependency survives
		found := false
		for _, it := range got.Initiatives {
			if w, ok := it.Work["Atlas"]; ok {
				for _, d := range w.DependsOn {
					if d == "Delta" {
						found = true
					}
				}
			}
		}
		Expect(found).To(BeTrue(), "expected Atlas -> Delta dependency to survive the write/read")
	})
})
