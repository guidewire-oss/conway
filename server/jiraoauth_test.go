package main

import (
	"conway/server/jira"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("pickJiraSite", func() {
	sites := []jira.Resource{
		{ID: "1", URL: "https://other-org.atlassian.net", Name: "Other"},
		{ID: "2", URL: "https://yourorg.atlassian.net", Name: "Your org"},
	}

	It("selects the site matching the hint", func() {
		Expect(pickJiraSite(sites, "yourorg").ID).To(Equal("2"))
	})

	It("falls back to the first resource when nothing matches", func() {
		Expect(pickJiraSite(sites, "no-such-site").ID).To(Equal("1"))
	})

	It("treats an empty hint (unconfigured CONWAY_JIRA_SITE_HINT) as the first resource", func() {
		Expect(pickJiraSite(sites, "").ID).To(Equal("1"))
	})
})
