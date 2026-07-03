package jira

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func ptr(t time.Time) *time.Time { return &t }

func TestAggregateStatsAndEdges(t *testing.T) {
	c := day("2026-01-01")
	issues := []Issue{
		// pod A: two resolved (5d, 15d) + one open
		{Key: "A-1", Pod: "Alpha", IssueType: "Story", Created: c, Resolved: ptr(c.AddDate(0, 0, 5))},
		{Key: "A-2", Pod: "Alpha", IssueType: "Story", Created: c, Resolved: ptr(c.AddDate(0, 0, 15))},
		{Key: "A-3", Pod: "Alpha", IssueType: "Story", Created: c, StatusCat: "indeterminate"}, // open -> wip
		// pod B: one resolved, plus an epic that must be excluded from cycle
		{Key: "B-1", Pod: "Beta", IssueType: "Story", Created: c, Resolved: ptr(c.AddDate(0, 0, 10))},
		{Key: "B-2", Pod: "Beta", IssueType: "Epic", Created: c, Resolved: ptr(c.AddDate(0, 0, 9))},
		// A-1 blocks B-1 -> one cross-pod edge Alpha->Beta
		{Key: "A-1b", Pod: "Alpha", IssueType: "Story", Created: c, Resolved: ptr(c.AddDate(0, 0, 5)), Blocks: []string{"B-1"}},
	}
	// fix: give the blocking issue a real key referenced by podByKey
	issues[5].Key = "A-1"

	stats, edges := Aggregate(issues, WipModeLeaf)

	a, ok := stats["Alpha"]
	if !ok {
		t.Fatal("no Alpha stats")
	}
	if a.WipCount != 1 {
		t.Fatalf("Alpha wip = %d, want 1", a.WipCount)
	}
	if a.ResolvedCount180d != 3 { // A-1, A-2, and the A-1 blocker dup-key resolved
		t.Fatalf("Alpha resolved = %d, want 3", a.ResolvedCount180d)
	}
	b := stats["Beta"]
	if b.ResolvedCount180d != 1 { // epic excluded
		t.Fatalf("Beta resolved = %d, want 1 (epic excluded)", b.ResolvedCount180d)
	}

	if len(edges) != 1 || edges[0].From != "Alpha" || edges[0].To != "Beta" || edges[0].Count != 1 {
		t.Fatalf("edges = %+v, want one Alpha->Beta", edges)
	}
}

func TestAggregateWipOnlyPodGetsDefaultEntry(t *testing.T) {
	stats, _ := Aggregate([]Issue{{Key: "X-1", Pod: "Gamma", IssueType: "Story", Created: day("2026-01-01"), StatusCat: "indeterminate"}}, WipModeLeaf)
	g, ok := stats["Gamma"]
	if !ok {
		t.Fatal("wip-only pod Gamma should still get a stats entry")
	}
	if g.WipCount != 1 || g.ResolvedCount180d != 0 || g.ThroughputPerWeek != 0 {
		t.Fatalf("Gamma default entry wrong: %+v", g)
	}
}

// TestAggregateWipExcludesBacklog reproduces the freeze-panel mismatch: a pod
// with a large unresolved backlog but nothing actually "In Progress" must
// report wip=0, matching the drill-down's status_cat='indeterminate' filter
// (server/db/snapshotquery.go WipPage/WipSummary) — not "anything unresolved".
func TestAggregateWipExcludesBacklog(t *testing.T) {
	stats, _ := Aggregate([]Issue{
		{Key: "K-1", Pod: "Kanata", IssueType: "Story", Created: day("2026-01-01"), StatusCat: "new"},          // backlog
		{Key: "K-2", Pod: "Kanata", IssueType: "Story", Created: day("2026-01-01"), StatusCat: "new"},          // backlog
		{Key: "K-3", Pod: "Kanata", IssueType: "Story", Created: day("2026-01-01"), StatusCat: "indeterminate"}, // actually in progress
	}, WipModeLeaf)
	k, ok := stats["Kanata"]
	if !ok {
		t.Fatal("no Kanata stats")
	}
	if k.WipCount != 1 {
		t.Fatalf("Kanata wip = %d, want 1 (backlog issues must not count as wip)", k.WipCount)
	}
}

// TestAggregateWipModeLeafExcludesEpics: an in-progress Epic must not count as
// wip alongside its in-progress children — mode "leaf" counts only non-epic
// issues, so both active children are counted and the epic container is not.
func TestAggregateWipModeLeafExcludesEpics(t *testing.T) {
	c := day("2026-01-01")
	stats, _ := Aggregate([]Issue{
		{Key: "E-1", Pod: "Delta", IssueType: "Epic", Created: c, StatusCat: "indeterminate"},
		{Key: "D-1", Pod: "Delta", IssueType: "Story", Created: c, StatusCat: "indeterminate", ParentKey: "E-1"},
		{Key: "D-2", Pod: "Delta", IssueType: "Story", Created: c, StatusCat: "indeterminate", ParentKey: "E-1"},
	}, WipModeLeaf)
	d := stats["Delta"]
	if d.WipCount != 2 {
		t.Fatalf("Delta wip = %d, want 2 (epic excluded, both children counted)", d.WipCount)
	}
}

// TestAggregateWipModeEpicOrParentless: the epic counts as one unit; its
// children (which have a parent epic) are not counted individually, but a
// parentless story/task still is.
func TestAggregateWipModeEpicOrParentless(t *testing.T) {
	c := day("2026-01-01")
	stats, _ := Aggregate([]Issue{
		{Key: "E-1", Pod: "Delta", IssueType: "Epic", Created: c, StatusCat: "indeterminate"},
		{Key: "D-1", Pod: "Delta", IssueType: "Story", Created: c, StatusCat: "indeterminate", ParentKey: "E-1"},
		{Key: "D-2", Pod: "Delta", IssueType: "Story", Created: c, StatusCat: "indeterminate", ParentKey: "E-1"},
		{Key: "D-3", Pod: "Delta", IssueType: "Story", Created: c, StatusCat: "indeterminate"}, // no epic parent
	}, WipModeEpicOrParentless)
	d := stats["Delta"]
	if d.WipCount != 2 {
		t.Fatalf("Delta wip = %d, want 2 (epic E-1 + parentless D-3; D-1/D-2 folded into their epic)", d.WipCount)
	}
}

func TestPodAlias(t *testing.T) {
	if podOf("Moose Factory") != "MooseFactory" {
		t.Fatal("alias not applied")
	}
}
