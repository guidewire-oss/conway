package main

import (
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Org Network double-clicks a from->to edge to drill into the Jira issues
// behind it; from/to are required so the query never runs against every
// issue link in the snapshot.
var _ = Describe("edge-issues", func() {
	DescribeTable("requires both from and to, answering 400 otherwise",
		func(from, to string) {
			s := &server{}
			req := httptest.NewRequest("GET", "/api/snapshots/x/edge-issues?from="+from+"&to="+to, nil)
			rec := httptest.NewRecorder()
			s.handleEdgeIssues(rec, req, "x")
			Expect(rec.Code).To(Equal(400), "from=%q to=%q", from, to)
		},
		Entry("both empty", "", ""),
		Entry("from only", "TeamA", ""),
		Entry("to only", "", "TeamB"),
	)
})
