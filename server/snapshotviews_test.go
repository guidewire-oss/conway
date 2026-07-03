package main

import (
	"net/http/httptest"
	"testing"
)

// Org Network double-clicks a from->to edge to drill into the Jira issues
// behind it; from/to are required so the query never runs against every
// issue link in the snapshot.
func TestHandleEdgeIssuesRequiresFromAndTo(t *testing.T) {
	cases := []struct{ from, to string }{
		{"", ""},
		{"TeamA", ""},
		{"", "TeamB"},
	}
	for _, c := range cases {
		s := &server{}
		req := httptest.NewRequest("GET", "/api/snapshots/x/edge-issues?from="+c.from+"&to="+c.to, nil)
		rec := httptest.NewRecorder()
		s.handleEdgeIssues(rec, req, "x")
		if rec.Code != 400 {
			t.Fatalf("from=%q to=%q: status = %d, want 400", c.from, c.to, rec.Code)
		}
	}
}
