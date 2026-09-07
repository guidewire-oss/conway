package evidence_test

import (
	"conway/server/evidence"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per specs/026-reliable-evidence-foundation.md:34
var _ = Describe("saved evidence policies", func() {
	// per specs/026-reliable-evidence-foundation.md:42
	It("distinguishes absent, fresh and stale evidence at the exact threshold", func() {
		Expect(evidence.Freshness(0, 24, 100)).To(Equal("No evidence"))
		Expect(evidence.Freshness(100, 24, 86499)).To(Equal("Fresh"))
		Expect(evidence.Freshness(100, 24, 86500)).To(Equal("Stale"))
	})
	// per specs/026-reliable-evidence-foundation.md:90
	DescribeTable("restricts persistent credential destinations", func(site string, valid bool) {
		err := evidence.ValidateSite(site)
		Expect(err == nil).To(Equal(valid))
	}, Entry("cloud", "https://atlas.atlassian.net", true), Entry("HTTP", "http://atlas.atlassian.net", false), Entry("suffix trick", "https://atlas.atlassian.net.evil.test", false), Entry("credentials", "https://user@atlas.atlassian.net", false), Entry("path", "https://atlas.atlassian.net/foo", false), Entry("port", "https://atlas.atlassian.net:443", false), Entry("local", "https://localhost", false))
	// per specs/026-reliable-evidence-foundation.md:56
	It("authenticates credentials and binds them to one source", func() {
		secret := []byte("durable fixture secret")
		sealed, err := evidence.Seal(secret, "one", []byte("private token"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(sealed)).NotTo(ContainSubstring("private token"))
		plain, err := evidence.Open(secret, "one", sealed)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(plain)).To(Equal("private token"))
		_, err = evidence.Open([]byte("different server key"), "one", sealed)
		Expect(err).To(HaveOccurred())
		_, err = evidence.Open(secret, "two", sealed)
		Expect(err).To(HaveOccurred())
		sealed[len(sealed)-1] ^= 1
		_, err = evidence.Open(secret, "one", sealed)
		Expect(err).To(HaveOccurred())
	})
	// per specs/026-reliable-evidence-foundation.md:61
	It("rejects ambiguous aliases instead of silently merging teams", func() {
		teams := []evidence.Team{{ID: "a", Name: "Atlas", Aliases: []string{"Atlas"}}, {ID: "b", Name: "Beacon", Aliases: []string{" atlas "}}}
		Expect(evidence.ValidateTeams(teams)).To(HaveOccurred())
		teams[1].Aliases = []string{"Beacon"}
		Expect(evidence.ValidateTeams(teams)).To(Succeed())
		Expect(evidence.TeamID(teams, " ATLAS ")).To(Equal("a"))
		Expect(evidence.TeamID(teams, "New team")).To(BeEmpty())
	})
	// per specs/026-reliable-evidence-foundation.md:61
	It("isolates identity namespaces and keeps provider identity independent of labels", func() {
		Expect(evidence.Identity("source-a", "issue", "123")).To(Equal(evidence.Identity("source-a", "issue", "123")))
		Expect(evidence.Identity("source-a", "issue", "123")).NotTo(Equal(evidence.Identity("source-b", "issue", "123")))
	})
	// per specs/026-reliable-evidence-foundation.md:89
	DescribeTable("accepts only the documented capture cadences", func(hours int, accepted bool) {
		config := evidence.Config{Name: "Atlas evidence", Site: "https://atlas.atlassian.net", Projects: []string{"PROJ"}, RosterID: "roster", WipMode: "leaf", IntervalHours: hours, FreshnessHours: 24, Teams: []evidence.Team{{ID: "team", Name: "Atlas", Aliases: []string{"Atlas"}}}}
		err := evidence.Validate(config)
		if accepted {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
	}, Entry("manual", 0, true), Entry("six hours", 6, true), Entry("daily", 24, true), Entry("weekly", 168, true), Entry("negative", -1, false), Entry("undocumented hourly", 1, false))
})
