package jira

import (
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	"time"
)

// HygieneStats computes typed per-pod data-quality aggregates from the issues
// (the source of the snapshot_pod_hygiene table). The heavy per-pod issue lists
// are no longer built here — they're queried from snapshot_issues on demand.
var _ = Describe("HygieneStats", func() {
	It("behaves", func() {
		now := day("2026-03-01")
		pts := func(f float64) *float64 { return &f }
		issues := []DetailedIssue{
			{Key: "A-1", IssueType: "Story", Pod: "Alpha", Summary: "sized", Points: pts(3),
				StatusName: "Done", StatusCat: "done", Created: day("2026-01-01"), Resolved: ptr(day("2026-01-10"))},
			{Key: "A-2", IssueType: "Story", Pod: "Alpha", Summary: "unsized open",
				StatusName: "To Do", StatusCat: "new", Created: day("2026-02-01")},
			// in-progress, stale (updated long ago) and unassigned
			{Key: "A-3", IssueType: "Story", Pod: "Alpha", Summary: "stale wip", Points: pts(2), StatusName: "In Progress",
				StatusCat: "indeterminate", Created: day("2026-01-01"), Updated: day("2026-01-01")},
		}
		stats := HygieneStats(issues, now)
		var alpha *HygieneStat
		for i := range stats {
			if stats[i].Pod == "Alpha" {
				alpha = &stats[i]
			}
		}
		if alpha == nil {
			Fail("no Alpha hygiene stat")
		}
		if alpha.SampleSized != 3 || alpha.WipCount != 1 {
			Fail(fmt.Sprintf("Alpha sampleSized=%d wipCount=%d, want 3/1", alpha.SampleSized, alpha.WipCount))
		}
		// 2 of 3 sized (A-1, A-3); the single WIP item is stale and unassigned
		if alpha.SizedPct == nil || *alpha.SizedPct < 0.66 || *alpha.SizedPct > 0.67 {
			Fail(fmt.Sprintf("Alpha sizedPct = %v, want ~0.667", alpha.SizedPct))
		}
		if alpha.StaleWipPct == nil || *alpha.StaleWipPct != 1 {
			Fail(fmt.Sprintf("Alpha staleWipPct = %v, want 1", alpha.StaleWipPct))
		}
		if alpha.UnassignedWipPct == nil || *alpha.UnassignedWipPct != 1 {
			Fail(fmt.Sprintf("Alpha unassignedWipPct = %v, want 1", alpha.UnassignedWipPct))
		}
	})
})

var _ = Describe("ADFTextLen", func() {
	It("behaves", func() {
		adf := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"},{"type":"text","text":" world"}]}]}`)
		if n := adfTextLen(adf); n != len("hello")+len("world") {
			Fail(fmt.Sprintf("adfTextLen = %d, want %d", n, len("hello")+len("world")))
		}
		if adfTextLen(json.RawMessage("null")) != 0 {
			Fail("null description should be len 0")
		}
	})
})

var _ = Describe("PodDevCounts", func() {
	It("behaves", func() {
		issues := []DetailedIssue{
			{Pod: "Alpha", Assignee: "Ann"},
			{Pod: "Alpha", Assignee: "Bob"},
			{Pod: "Alpha", Assignee: "Ann"}, // dup assignee
			{Pod: "Beta", Assignee: ""},     // unassigned → counts pod, 0 devs
			{Pod: "", Assignee: "Zed"},      // no pod → ignored
		}
		dc := PodDevCounts(issues)
		if dc["Alpha"] != 2 {
			Fail(fmt.Sprintf("Alpha devs = %d, want 2", dc["Alpha"]))
		}
		if dc["Beta"] != 0 {
			Fail(fmt.Sprintf("Beta devs = %d, want 0", dc["Beta"]))
		}
		if _, ok := dc[""]; ok {
			Fail("empty pod should be ignored")
		}
	})
})

var _ = Describe("ParseJiraTimeUsedByDetail", func() {
	It("behaves", func() {
		if _, ok := parseJiraTime("2026-06-13T03:22:11.123-0400"); !ok {
			Fail("should parse jira datetime")
		}
	})
})

var _ = time.Now
