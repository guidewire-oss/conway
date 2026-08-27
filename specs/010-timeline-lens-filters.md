# Timeline Lens Filters: fuzzy search by initiative and by pod

**Status:** In Progress
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-26
**Story/Ticket:** user request, post spec-009
**Sprint/Cycle:** n/a

---

## 1. Overview

Two filter inputs on the timeline: in the **by-pod** lens, a fuzzy search by
initiative that dims every non-matching bar across all pods — the planner sees
how one initiative lines up across teams. In the **by-initiative** lens, the
mirror: a fuzzy search by pod that dims slices not touching the typed pod and
collapses rows with no visible work.

---

## 2. Problem

Answering "where does Apollo run across the org?" means scanning 35 pods'
lanes by eye. Answering "what lands on Atlas?" means reading every initiative
row. Both are one fuzzy query's worth of work.

---

## 3. User Stories

### Story 1: Trace an initiative across pods

**As a** planning manager
**I want** to type (part of) an initiative name in the by-pod lens
**So that** its bars stay lit across every pod while everything else dims

### Story 2: Find what lands on a pod

**As a** planning manager
**I want** to type (part of) a pod name in the by-initiative lens
**So that** only the initiatives touching that pod stay readable

---

## 4. Acceptance Criteria

**AC 1.1: Fuzzy match, case-insensitive, substring-or-subsequence**

> Given the query "apollo"
> When the by-pod lens renders
> Then "Apollo/App Platform" matches; its bars render in every pod it
> touches and non-matching bars do not render at all

**AC 1.2: Empty query is no filter**

> Given an empty query
> Then every bar renders normally

**AC 2.1: Pod filter collapses empty rows**

> Given the pod query "atlas"
> When the by-initiative lens renders
> Then rows whose slices touch Atlas stay with those slices lit; other rows
> render dimmed or collapsed; the row count of full-opacity rows is visible

**AC 2.3: The waterfall reads as a chain**

> Given an initiative query in the by-pod lens
> When the pods render
> Then matching pods order by earliest matching start, then finish — the
> chain's earliest team on top, its dependents below; with "hide empty pods"
> checked, only the chain shows

**AC 2.2: Lens switches clear the filter**

> Given a filter set in one lens
> When switching lenses
> Then the filter clears — the query means a different thing per lens, and a
> carried-over initiative query in the pod filter (or vice versa) matches
> nothing, which reads as broken. (Amended after live testing: persistence
> was the original intent, but the cross-lens carryover surprised.)

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The by-pod lens MUST offer a fuzzy initiative filter; non-matching bars do not render, matching bars render in every pod they touch | MUST |
| FR-002 | The by-initiative lens MUST offer a fuzzy pod filter; rows without matching slices do not render | MUST |
| FR-003 | Matching MUST be case-insensitive substring or subsequence ("aplat" matches "Apollo/Mobile") | MUST |
| FR-004 | Filters are view state (clear on lens switch and plan switch), never persisted | MUST |
| FR-005 | Each filter input MUST show its live match count ("3 of 29 initiatives") | MUST |
| FR-006 | The by-pod lens MUST order matching pods as a waterfall: earliest matching start first, then earliest finish — the initiative's chain reads top-to-bottom | MUST |
| FR-007 | The by-pod lens MUST offer a "hide empty pods" checkbox that removes pods with no matching initiatives entirely | MUST |
| FR-008 | The filter input MUST keep focus and caret position across the re-renders its own keystrokes trigger | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression | suites green | `node --test`, `go test ./...` |
| NFR-002 | Filter re-render on 35-pod plan | < 100ms per keystroke | in-browser |

---

## 7. Data Model

None — view state only (`current.tlFilter`).

---

## 8. API Contract

None.

---

## 9. Out of Scope

- Filtering the Order table (different view, different pass)
- Saved/named filters

---

## 10. Open Questions

None.

---

## 11. Decision Record

### Decision 1: Hide, do not dim (amended 2026-08-26 on the product owner's call)

**Context:** The first implementation dimmed non-matching bars to 25% to keep
the capacity context. Live use said otherwise: the dimmed crowd read as noise,
and the isolated track across pods — with its lanes and dependencies — IS the
picture the filter exists to draw.

**Decision:** Non-matching bars do not render at all, in both lenses. Under a
filter the matching pods WATERFALL — earliest matching start first — so the
initiative's chain reads top-to-bottom, and a "hide empty pods" checkbox
removes the pods it doesn't touch. The match count keeps the denominator
honest ("1 of 29").

**Alternatives considered:**
- Dim at 25% — rejected in review with the product owner: noise, not context.

### Decision 2: The pod filter mirrors into the by-initiative lens (the user's own suggestion)

**Context:** Symmetric questions deserve symmetric tools: "where does this
initiative run?" and "what runs on this pod?" are the same query from two
sides.

**Decision:** Both lenses get a filter input, each scoped to the other axis.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Gestures to trace an initiative across pods | visual scan of 35 pods | one query | in-browser |
