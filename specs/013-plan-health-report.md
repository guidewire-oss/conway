# Plan Health Report

**Status:** Draft
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-30
**Story/Ticket:** user-directed polish ("one page a manager can take to a meeting")
**Sprint/Cycle:** n/a

---

## 1. Overview

One printable card that answers "is my plan OK, and what would fix it" in
under a minute: the verdict distribution, the initiatives that will not fit
the period, the pods that cannot carry the load, the date-locked conflicts,
and the top remedies the engine computed. Every number on the card is the
same engine output the Order and Timeline views render — the report is a
summarizing lens, never a second computation.

---

## 2. Problem

The plan's health is scattered: verdicts live in the Order table, capacity in
the Timeline, conflicts and remedies behind per-row expanders, baselines in a
chip. A manager preparing for a review must screenshot four views and do the
arithmetic themselves — so most walk in with nothing.

Concrete example: a 29-initiative portfolio planned into a 26-week period
has six initiatives finishing past the horizon and two structurally
infeasible. Nothing in the app states "6 of 29 will not finish inside the
period, here are they, here is what holding three levers would do" — the
single sentence the meeting needs.

---

## 3. User Stories

### Story 1: Read the plan's health in one glance

**As a** planning manager
**I want** one card with the verdict counts, the problem initiatives, the
hottest pods, and the top remedies
**So that** I can open it in a review and speak to the plan's state without
navigating four views.

### Story 2: Take the report to the meeting

**As a** planning manager
**I want** to print the card (or save it as PDF) on one page
**So that** stakeholders who never open Conway still see the plan's state.

### Story 3: Trust every number on it

**As a** planning manager
**I want** the report computed from the same schedule the other views render
**So that** a number in the report can never contradict the Order view.

---

## 4. Acceptance Criteria

### Story 1: One-glance health

**AC 1.1: The verdict distribution is shown as counts**

> Given a computed schedule
> When the report opens
> Then each verdict (on-time, late, beyond-horizon, structurally-infeasible,
> unschedulable, no-date) shows its initiative count
> And the problem verdicts list the initiative names, not just counts.

**AC 1.2: The headline sentence states the fit**

> Given a schedule where some initiatives commit past the horizon or cannot
> be scheduled
> When the report opens
> Then the first line states how many of the total initiatives will not
> finish inside the period
> And an all-green plan states that everything commits inside the period.

**AC 1.3: Capacity pressure is named per pod**

> Given the schedule's per-pod utilization
> When the report opens
> Then pods at ρ≥1 and ρ≥0.85 are listed with their ρ and track count
> And the drum (constraint) pods are marked as such.

**AC 1.4: Conflicts are surfaced or their absence is stated**

> Given date-locked initiatives contending for the same pod
> When the report opens
> Then each conflict pair is listed
> And a report with none states that there are no date conflicts.

**AC 1.5: The top remedies are offered with their cost**

> Given the remedies the engine computed for this schedule
> When the report opens
> Then the top remedies by portfolio objective improvement are shown with
> the target, the resulting verdict, and how many other initiatives move
> And each links back to the Order view's remedy panel for the full picture.

### Story 2: One printed page

**AC 2.1: Printing yields the card alone**

> Given the report is open
> When the planner prints (or saves as PDF)
> Then only the report card is printed — no tabs, no tables behind it
> And the card fits one page for a plan of ~30 initiatives and ~35 pods.

**AC 2.2: Baseline context is on the card**

> Given an active baseline
> When the report opens
> Then the card names the active baseline and when it was saved
> And a plan without baselines says so.

### Story 3: One source of truth

**AC 3.1: No independent arithmetic**

> Given any figure on the card
> When the report renders
> Then the figure comes from the rendered schedule or the remedies response —
> the report computes no schedule, rho, or verdict of its own.

**AC 3.2: Staleness is honest**

> Given the schedule changed after the report was opened
> When the underlying plan is recomputed
> Then the open report is closed or re-rendered from the new schedule — never
> left showing the old one as if current.

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The plan views row MUST offer a Report affordance that opens the card | MUST |
| FR-002 | The card MUST render verdict counts with problem-initiative names | MUST |
| FR-003 | The card MUST open with the headline fit sentence (AC 1.2) | MUST |
| FR-004 | The card MUST list hot and over-capacity pods with ρ and mark drum pods | MUST |
| FR-005 | The card MUST show date conflicts, or state there are none | MUST |
| FR-006 | The card MUST fetch top remedies on open and list them with resulting verdict and moved-initiative count | SHOULD |
| FR-007 | Printing MUST isolate the card (print stylesheet), fitting one page | MUST |
| FR-008 | The card MUST name the active baseline or state there is none | MUST |
| FR-009 | Every figure MUST come from the rendered schedule or the remedies response (AC 3.1) | MUST |
| FR-010 | A plan with no schedule (nothing computed yet) MUST state that and offer nothing else | MUST |
| FR-011 | The card SHOULD carry the generated-at timestamp and the dispatch rule used | SHOULD |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | Report opens from the cached schedule | no schedule recompute on open | network log |
| NFR-002 | Printed card fits one A4 page | ~30 initiatives / ~35 pods | manual print |
| NFR-003 | Remedies fetch failure degrades to a named note | card still renders | simulate 500 |

---

## 7. Data Model

No new entities. The card reads:

- The rendered schedule (`/api/plan/{id}/schedule` response): initiatives
  (verdict, weeksLate, commitWeek), podWeeks (flatRho, tracks, idle),
  drumPods, fit, rule, objectiveScore, horizonWeeks, periodStart.
- The remedies response (`POST /api/plan/{id}/schedule/remedies`): remedies
  (kind, target, resultingVerdict, objectiveDelta, affectedInitiatives),
  warnings, conflicts.
- Baseline metadata already cached on the plan (active baseline, saved-at).

---

## 8. API Contract

No new endpoints. The report consumes the existing schedule and remedies
endpoints.

---

## 9. Out of Scope

- Server-side PDF generation (the browser prints)
- Scheduled/emailed reports
- Actuals and variance on the card (spec 001 Story 10 — a later section once
  actuals exist)
- Editing anything from the card (the card links to the views that edit)

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should the remedies fetch run on every open (fresh engine runs, slower) or only on explicit request (fast open, stale-free but empty by default)? | Anoop | 2026-09-05 | pending — default: fetch on open; the Order view already does per-click remedies |
| Q2 | Should the report also appear as a downloadable standalone HTML file, or is browser print enough? | Anoop | 2026-09-05 | pending — default: browser print only |

---

## 11. Decision Record

### Decision 1: The report is a summarizing lens over the one schedule, not a new computation

**Context:** A report that recomputes verdicts or utilization client-side
becomes a second source of truth that can disagree with the Order view — the
exact drift the drag-to-edit work (spec 008 Decision 1) eliminated by
re-routing every change through the engine.

**Decision:** `healthReportHTML` is a pure string function over the rendered
schedule object and the remedies response. It computes only aggregations
(counts, sorting, top-N) — never a verdict, a ρ, or a schedule.

**Alternatives considered:**
- A server report endpoint that recomputes — rejected: a second render path
  to keep in sync, and it would freeze the report's schedule at fetch time
  while the views move on.

**Consequences:** The report is as current as the views behind it and cannot
disagree with them by construction. Opening it is free (cached schedule);
only the remedies section costs an engine round trip.

### Decision 2: Print isolation via a print stylesheet, not a separate page

**Context:** A standalone report route would need its own auth, navigation,
and state wiring for what is visually one card.

**Decision:** The card opens in an overlay and a `@media print` stylesheet
hides everything else, so browser print/PDF yields the card alone.

**Alternatives considered:**
- A standalone `/report` page — rejected: duplicate plumbing for one page of
  content; revisit if scheduled/emailed reports ever land (out of scope).

**Consequences:** Printing works on any plan without new state; the overlay
reuses the app's modal patterns (ESC to close, focus management).

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Time to answer "is the plan OK" in a review | ~4 views + manual arithmetic | 1 open + 1 print | in-browser |
| Numbers that can contradict the Order view | possible (manual) | 0 (one source) | by construction |
| Plans taken to meetings as PDF | 0 | adoption | user feedback |
