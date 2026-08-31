package planning

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Spec 003: the site timezone overlap engine. Table-driven at two dates
// across a DST boundary (NFR-002), because the whole point of IANA zones over
// fixed offsets is that the answer changes when the clocks do.

var _ = Describe("OverlapHours", func() {
	dublin := Site{Name: "Dublin", Timezone: "Europe/Dublin"}
	warsaw := Site{Name: "Warsaw", Timezone: "Europe/Warsaw"}
	denver := Site{Name: "Denver", Timezone: "America/Denver"}
	kolkata := Site{Name: "Bengaluru", Timezone: "Asia/Kolkata"}
	unset := Site{Name: "Nowhere"}

	// Winter: Dublin/Warsaw UTC+0/+1, Denver UTC-7. Summer: Dublin +1, Denver -6.
	summer := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	It("gives two sites in the same timezone a full working day (AC 1.1)", func() {
		hours, configured, err := OverlapHours(warsaw, Site{Name: "Krakow", Timezone: "Europe/Warsaw"}, summer)
		Expect(err).NotTo(HaveOccurred())
		Expect(hours).To(Equal(8.0))
		Expect(configured).To(BeTrue())
	})

	It("gives a same-site pair a full day without any table entry (FR-005)", func() {
		hours, configured, err := OverlapHours(dublin, dublin, summer)
		Expect(err).NotTo(HaveOccurred())
		Expect(hours).To(Equal(8.0))
		Expect(configured).To(BeTrue())
	})

	It("prices Dublin–Warsaw far above Denver–Warsaw at the same date (the spec's own example)", func() {
		dw, c1, err := OverlapHours(dublin, warsaw, winter)
		Expect(err).NotTo(HaveOccurred())
		Expect(c1).To(BeTrue())
		dwv, c2, err := OverlapHours(denver, warsaw, winter)
		Expect(err).NotTo(HaveOccurred())
		Expect(c2).To(BeTrue())
		Expect(dw).To(BeNumerically(">", 6), "one hour apart share most of a day")
		Expect(dwv).To(BeNumerically("<", 1), "seven hours apart share the fringe")
	})

	It("changes the answer across a DST boundary (NFR-002)", func() {
		// Dublin observes DST; Kolkata does not — so the same two configured
		// sites have a different overlap in July than in January.
		summerH, _, err := OverlapHours(dublin, kolkata, summer)
		Expect(err).NotTo(HaveOccurred())
		winterH, _, err := OverlapHours(dublin, kolkata, winter)
		Expect(err).NotTo(HaveOccurred())
		Expect(summerH).NotTo(Equal(winterH), "Dublin's window shifts an hour; Kolkata's never moves")
		Expect(summerH).To(Equal(3.5), "July: Dublin UTC+1 -> 08:00-16:00 UTC vs Kolkata 03:30-11:30")
		Expect(winterH).To(Equal(2.5), "January: Dublin UTC+0 -> 09:00-17:00 UTC vs Kolkata 03:30-11:30")
	})

	It("finds zero overlap between Kolkata and Denver whatever the season", func() {
		hours, configured, err := OverlapHours(kolkata, denver, summer)
		Expect(err).NotTo(HaveOccurred())
		Expect(configured).To(BeTrue())
		Expect(hours).To(Equal(0.0))
	})

	It("resolves an unconfigured site to not-computed (Decision 3)", func() {
		_, configured, err := OverlapHours(unset, warsaw, summer)
		Expect(err).NotTo(HaveOccurred())
		Expect(configured).To(BeFalse())
	})

	It("refuses a timezone the tz database does not know", func() {
		_, _, err := OverlapHours(Site{Name: "X", Timezone: "Mars/Olympus"}, warsaw, summer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Mars/Olympus"))
	})
})

var _ = Describe("HandoffLatencyDays", func() {
	// The bands SPEC.md defines (AC 1.2), unchanged.
	bands := []struct {
		hours    float64
		config   bool
		expected float64
		note     string
	}{
		{8, true, 0.25, "a full shared day is still the best band"},
		{4, true, 0.25, "the quarter-day boundary"},
		{3, true, 0.5, "half-day band"},
		{1, true, 1.0, "any overlap: full day"},
		{0, true, 1.5, "none: day and a half"},
		{0, false, 1.5, "unconfigured: the same pessimistic default (Decision 3)"},
	}
	for _, b := range bands {
		b := b
		It("maps "+b.note, func() {
			Expect(HandoffLatencyDays(b.hours, b.config)).To(Equal(b.expected))
		})
	}
})
