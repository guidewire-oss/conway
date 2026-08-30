# Baseline-to-Baseline Comparison

**Status:** In Progress
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-25
**Story/Ticket:** follows specs/001 Story 7 (AC 7.4); discovered during portfolio-plan testing
**Sprint/Cycle:** n/a

---

## 1. Overview

Today a baseline can only be compared against the live order on screen. The
question leadership actually asks is "what changed between the agreed v1 and
the proposed v2?" — and answering it today means comparing each baseline to
the current order separately and eyeballing two cards. This spec adds
direct baseline-to-baseline comparison.

---

## 2. Problem

A plan carries multiple named baselines by design (spec 001 Story 7: freeze
an accepted order, later re-plans are measured against it). When a second
baseline is saved after inputs move, the delta between the two baselines is
the decision artefact — "since we agreed v1, these initiatives moved by this
much" — but no endpoint or view computes it. Both schedules are already
stored in full inside the baseline rows; nothing new needs capturing.

---

## 3. User Stories

### Story 1: Compare two saved baselines

**As a** planning manager
**I want** to pick two saved baselines and see their delta
**So that** I can present what changed between the agreed order and the
proposed one, without reverting plan inputs to reconstruct either side

---

## 4. Acceptance Criteria

**AC 1.1: The delta is computed from the stored schedules**

> Given a plan with two saved baselines A and B
> When the planner compares A against B
> Then the response carries the same BaselineComparison shape the live
> compare uses — per initiative: rank, start, commit, verdict in both, and
> the deltas — plus additions and removals

**AC 1.2: The picker refuses nonsense**

> Given fewer than two saved baselines
> When the compare control is rendered
> Then it is absent or disabled (nothing to compare)

**AC 1.3: Either direction, clearly labelled**

> Given baselines v1 and v2
> When the planner compares v1 → v2
> Then the card names both ("since v1 first cut → v2 Aurora first") so the
> reader knows which way the deltas read

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The system MUST offer `POST /api/plan/{id}/baseline/{bid}/compare-to/{other}` returning the baseline-to-baseline comparison | MUST |
| FR-002 | The comparison MUST reuse `CompareToBaseline` so both compare flavours report identical shapes | MUST |
| FR-003 | The baseline panel MUST offer pairwise compare when two or more baselines exist, and MUST NOT when fewer | MUST |
| FR-004 | The comparison card MUST label both baselines by name | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression | existing suites green | `go test ./...`, `node --test` |
| NFR-002 | Comparison of two 35-initiative baselines | < 1s end-to-end, as observed in the browser | in-browser timing |

NFR-002 is deliberately an end-to-end budget rather than a server-side one, because
in-browser timing is what was actually measured and it includes network transfer and
rendering. Isolating the handler would need a Go benchmark against a seeded
database; that has not been run, so no server-side figure is claimed here. If the
end-to-end budget is ever missed, that benchmark is the way to find out which half
is responsible.

---

## 7. Data Model

No changes. Baseline rows already store the full schedule blob.

---

## 8. API Contract

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| POST | /api/plan/{id}/baseline/{bid}/compare-to/{other} | Compare stored baseline `{bid}` against stored baseline `{other}` | — | `{from: {...}, to: {...}, comparison: BaselineComparison}` |

---

## 9. Out of Scope

- Comparing more than two baselines at once
- Diffing the INPUTS (roster, estimates) between baselines — schedules only
- Actuals (specs/001 Story 10, blocked on Jira)

---

## 10. Open Questions

None.

---

## 11. Decision Record

### Decision 1: Reuse CompareToBaseline, no new delta engine

**Context:** The live compare already computes rank/start/commit/verdict
deltas between two `Schedule` values.

**Decision:** The new endpoint unmarshals both stored schedules and calls the
same function. Direction is **positional, never inferred**: `{bid}` is always the
`from` end and `{other}` always the `to` end, so deltas run from the first id in
the URL toward the second whatever their relative age. Passing a newer baseline as
`{bid}` is legal and reports the movement backwards; nothing in the handler
compares timestamps or ids to reorder the pair. The UI builds the URL from the row
the select sits on, which makes that row the `from` end.

**Alternatives considered:**
- A bespoke pairwise diff with its own shape — rejected: two comparison
  flavours with different shapes is drift the UI would pay for forever.

**Consequences:** Whatever `CompareToBaseline` reports (or forgets to
report) is consistent everywhere.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Baseline comparisons possible | live-vs-one only | any pair | in-browser |
