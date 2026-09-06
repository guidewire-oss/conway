package sheets_test

import (
	"conway/server/planning"
	"conway/server/sheets"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("linked Sheets captures", func() {
	var current planning.BaselineInputs
	matrix := func(estimate string) [][]string {
		return [][]string{{"Initiative", "Full Kit Estimate", "Team A Dependencies", "Team A"}, {"Atlas", "4", "NONE", estimate}}
	}
	BeforeEach(func() {
		current = planning.NewBaselineInputs([]planning.Team{{Name: "Team A", Tracks: 2}, {Name: "Team B", Tracks: 1}}, []planning.Initiative{{Name: "Atlas", Work: map[string]planning.TeamWork{"Team A": {Weeks: 4, Estimated: true, InPath: true}}, PinnedStarts: map[string]int{"Team A": 1}, PinnedLanes: map[string]int{"Team A": 0}, EpicKeys: []string{"PROJ-1"}}}, planning.Params{HorizonWeeks: 26}, planning.SchedulingParams{})
	})
	It("retains explicit unknown effort, matching identities, pins and omitted epic bindings", func() {
		rows := matrix("TBD")
		rows[0][3] = " team a "
		rows[1][0] = " atlas "
		before := current.Fingerprint()
		got := sheets.Parse("initiatives", rows, current)
		Expect(got.Valid()).To(BeTrue(), "%v", got.Errors)
		Expect(got.Count).To(Equal(1))
		Expect(got.Warnings).NotTo(BeEmpty())
		it := got.Initiatives[0]
		Expect(it.Name).To(Equal("Atlas"))
		Expect(it.Work["Team A"].InPath).To(BeTrue())
		Expect(it.Work["Team A"].Estimated).To(BeFalse())
		Expect(it.PinnedStarts).To(Equal(current.Initiatives[0].PinnedStarts))
		Expect(it.PinnedLanes).To(Equal(current.Initiatives[0].PinnedLanes))
		Expect(it.EpicKeys).To(Equal([]string{"PROJ-1"}))
		Expect(current.Fingerprint()).To(Equal(before))
		Expect(rows[0][3]).To(Equal(" team a "))
	})
	DescribeTable("refuses malformed effort rather than silently losing assignments", func(value string) {
		got := sheets.Parse("initiatives", matrix(value), current)
		Expect(got.Valid()).To(BeFalse())
		Expect(got.Errors).NotTo(BeEmpty())
	}, Entry("unknown text", "later"), Entry("negative", "-2"), Entry("infinity", "Inf"), Entry("not a number", "NaN"))
	DescribeTable("preserves accepted effort units without silently clearing work", func(value string) {
		got := sheets.Parse("initiatives", matrix(value), current)
		Expect(got.Valid()).To(BeTrue(), "%v", got.Errors)
		Expect(got.Initiatives[0].Work["Team A"]).To(Equal(planning.TeamWork{Weeks: 4, Estimated: true, InPath: true}))
	}, Entry("short week suffix", "4wk"), Entry("plural week suffix", "4wks"))
	DescribeTable("accepts only supported requester tiers or blank", func(value string, want int) {
		got := sheets.Parse("initiatives", [][]string{{"Initiative", "Tier", "Full Kit Estimate", "Team A"}, {"Atlas", value, "4", "4"}}, current)
		Expect(got.Valid()).To(BeTrue(), "%v", got.Errors)
		Expect(got.Initiatives[0].Tier).To(Equal(want))
	}, Entry("blank unset", "", 0), Entry("tier one", "1", 1), Entry("tier two", "2", 2), Entry("tier three", "3", 3), Entry("tier four", "4", 4))
	DescribeTable("rejects populated requester tiers outside integers one through four", func(value string) {
		got := sheets.Parse("initiatives", [][]string{{"Initiative", "Tier", "Full Kit Estimate", "Team A"}, {"Atlas", value, "4", "4"}}, current)
		Expect(got.Valid()).To(BeFalse())
		Expect(got.Errors).NotTo(BeEmpty())
	}, Entry("zero", "0"), Entry("above range", "5"), Entry("negative", "-1"), Entry("fraction", "2.5"), Entry("text", "urgent"))
	DescribeTable("rejects conflicting roster header aliases", func(first, second, a, b string) {
		got := sheets.Parse("teams", [][]string{{"Name", first, second}, {"Team A", a, b}}, current)
		Expect(got.Valid()).To(BeFalse())
		Expect(got.Errors).NotTo(BeEmpty())
	}, Entry("capacity aliases", "Tracks", "Capacity", "2", "8"), Entry("location aliases", "Site", "Location", "North", "South"))
	It("refuses unknown populated team columns and unresolved dependencies", func() {
		rows := matrix("4")
		rows[0][3] = "Unknown Team"
		Expect(sheets.Parse("initiatives", rows, current).Valid()).To(BeFalse())
		rows = matrix("4")
		rows[1][2] = "Unknown Team"
		Expect(sheets.Parse("initiatives", rows, current).Valid()).To(BeFalse())
	})
	It("refuses duplicate normalized initiative identities and data outside headers", func() {
		rows := matrix("4")
		rows = append(rows, []string{" atlas ", "3", "NONE", "3"})
		Expect(sheets.Parse("initiatives", rows, current).Valid()).To(BeFalse())
		rows = matrix("4")
		rows[1] = append(rows[1], "unmapped work")
		Expect(sheets.Parse("initiatives", rows, current).Valid()).To(BeFalse())
	})

	DescribeTable("refuses malformed populated planning metadata", func(header, value string) {
		rows := [][]string{{"Initiative", header, "Full Kit Estimate", "Team A"}, {"Atlas", value, "4", "4"}}
		got := sheets.Parse("initiatives", rows, current)
		Expect(got.Valid()).To(BeFalse(), "populated %s=%q was silently accepted: %#v", header, value, got.Initiatives)
		Expect(got.Errors).NotTo(BeEmpty())
	}, Entry("priority", "Priority", "urgent"), Entry("target date", "Target Date", "next quarter"), Entry("cost of delay", "Cost of Delay", "many"), Entry("full-kit readiness", "Kit %", "nearly"))
	It("identifies explicit assigned-work removal for review", func() {
		got := sheets.Parse("initiatives", matrix("No Dependency"), current)
		Expect(got.Removals).NotTo(BeEmpty())
		Expect(got.Warnings).NotTo(BeEmpty())
	})
	It("does not accept an empty capture as a replacement", func() {
		for _, rows := range [][][]string{nil, {{"Name", "Tracks"}}, {{"Name", "Tracks"}, {"", ""}}} {
			got := sheets.Parse("teams", rows, current)
			Expect(got.Valid()).To(BeFalse())
			Expect(got.Errors).NotTo(BeEmpty())
		}
	})
	It("retains the opposite input kind and rejects removing a team required by current work", func() {
		got := sheets.Parse("teams", [][]string{{"Name", "Tracks"}, {" team a ", "3"}, {"Team B", "1"}}, current)
		Expect(got.Valid()).To(BeTrue(), "%v", got.Errors)
		Expect(got.Teams[0].Name).To(Equal("Team A"))
		Expect(got.Initiatives).To(Equal(current.Initiatives))
		got = sheets.Parse("teams", [][]string{{"Name", "Tracks"}, {"Team B", "1"}}, current)
		Expect(got.Valid()).To(BeFalse())
		Expect(got.Removals).To(ContainElement("Team A"))
	})
	It("rejects malformed roster values and duplicate normalized names", func() {
		for _, rows := range [][][]string{
			{{"Name", "Tracks"}, {"Team A", "many"}},
			{{"Name", "Pairs"}, {"Team A", "perhaps"}},
			{{"Name", "Capacity loss"}, {"Team A", "100%"}},
			{{"Name", "Tracks"}, {"Team A", "1"}, {" team a ", "2"}},
			{{"Name", "Unknown field"}, {"Team A", "work"}},
		} {
			Expect(sheets.Parse("teams", rows, current).Valid()).To(BeFalse())
		}
	})
})

var _ = Describe("linked Sheet URL validation", func() {
	It("extracts an ID from the canonical HTTPS Google URL", func() {
		id, err := sheets.ParseLink("https://docs.google.com/spreadsheets/d/generic-sheet-id/edit#gid=0")
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal("generic-sheet-id"))
	})
	DescribeTable("rejects arbitrary network targets", func(raw string) { _, err := sheets.ParseLink(raw); Expect(err).To(HaveOccurred()) },
		Entry("HTTP", "http://docs.google.com/spreadsheets/d/generic-sheet-id/edit"),
		Entry("lookalike host", "https://docs.google.com.example.com/spreadsheets/d/generic-sheet-id/edit"),
		Entry("userinfo", "https://user@docs.google.com/spreadsheets/d/generic-sheet-id/edit"),
		Entry("local target", "https://127.0.0.1/spreadsheets/d/generic-sheet-id/edit"),
		Entry("other Google document", "https://docs.google.com/document/d/generic-sheet-id/edit"))
})
