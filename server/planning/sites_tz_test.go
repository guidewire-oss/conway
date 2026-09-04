package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Spec 003 review: inference from the site's location — unambiguous cities
// infer silently, ambiguous cities (Birmingham: UK or Alabama) are flagged
// needsConfirm with their candidates, never silently guessed.
var _ = Describe("site timezone inference", func() {
	It("infers unambiguous cities silently", func() {
		Expect(inferTimezone("Bengaluru")).To(Equal("Asia/Kolkata"))
		Expect(inferTimezone("Kraków")).To(Equal("Europe/Warsaw"))
		Expect(inferTimezone("San Mateo")).To(Equal("America/Los_Angeles"))
	})

	It("flags ambiguous cities with candidates and no guess", func() {
		tz, candidates := inferTimezone("Birmingham")
		Expect(tz).To(Equal(""))
		Expect(candidates).To(Equal([]string{"Europe/London", "America/Chicago"}))
	})

	It("infers nothing for remote decorations or unknown names", func() {
		tz, cands := inferTimezone("*REMOTE - multicontinental*")
		Expect(tz).To(Equal(""))
		Expect(cands).To(BeEmpty())
		Expect(inferTimezone("Atlantis")).To(Equal(""))
	})

	It("seeds sites: roster-set timezone wins, then inference, then needsConfirm", func() {
		teams := []Team{
			{Name: "Alpha", Site: "Bengaluru"},                         // infers
			{Name: "Beta", Site: "Birmingham"},                         // ambiguous
			{Name: "Gamma", Site: "Krakow", Timezone: "Europe/Warsaw"}, // roster-set
		}
		sites := SitesFromTeams(teams, nil)
		byName := map[string]Site{}
		for _, st := range sites {
			byName[st.Name] = st
		}
		Expect(byName["Bengaluru"].Timezone).To(Equal("Asia/Kolkata"))
		Expect(byName["Bengaluru"].Inferred).To(BeTrue())
		Expect(byName["Birmingham"].Timezone).To(Equal(""))
		Expect(byName["Birmingham"].NeedsConfirm).To(BeTrue())
		Expect(byName["Birmingham"].Candidates).To(Equal([]string{"Europe/London", "America/Chicago"}))
		Expect(byName["Krakow"].Timezone).To(Equal("Europe/Warsaw"), "roster-set wins")
		Expect(byName["Krakow"].Inferred).To(BeFalse())
	})

	It("keeps existing configured sites when re-seeding the same roster", func() {
		teams := []Team{{Name: "Alpha", Site: "Bengaluru"}}
		existing := []Site{{Name: "Bengaluru", Timezone: "America/Blah", Unknown: false}}
		sites := SitesFromTeams(teams, existing)
		Expect(sites).To(HaveLen(1))
		Expect(sites[0].Timezone).To(Equal("America/Blah"), "admin config wins over inference")
	})
})
