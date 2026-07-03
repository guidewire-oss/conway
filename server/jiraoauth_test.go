package main

import (
	"testing"

	"conway/jira"
)

func TestPickJiraSite(t *testing.T) {
	sites := []jira.Resource{
		{ID: "1", URL: "https://other-org.atlassian.net", Name: "Other"},
		{ID: "2", URL: "https://yourorg.atlassian.net", Name: "Your org"},
	}

	if got := pickJiraSite(sites, "yourorg"); got.ID != "2" {
		t.Fatalf("hint should select the matching site, got %+v", got)
	}
	if got := pickJiraSite(sites, "no-such-site"); got.ID != "1" {
		t.Fatalf("no match should fall back to the first resource, got %+v", got)
	}
	if got := pickJiraSite(sites, ""); got.ID != "1" {
		t.Fatalf("empty hint (unconfigured CONWAY_JIRA_SITE_HINT) should use the first resource, got %+v", got)
	}
}
