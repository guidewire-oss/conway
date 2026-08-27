# Planning Page IA Pass 2: Results First, Decisions Surfaced

**Status:** In Progress
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-26
**Story/Ticket:** UX/IA audit of the planning page, post spec-008
**Sprint/Cycle:** n/a

---

## 1. Overview

An end-to-end UX walk of the planning experience (plans list → plan → Network/
Order/Timeline) found the page still leads with configuration rather than
results, the baseline record is four screens below the fold, three model
choices demand decisions without framing them as decisions, and several
warning states dead-end without actions. This spec is the second IA pass over
the planning page: rearrange what exists, make copy actionable, and surface
the plan's health — no new engine work.

---

## 2. Problem

**Config-first layout.** Opening a plan shows horizon/loss inputs, roster
switcher, upload buttons, a strict-deps checkbox and sample links before any
result. A returning planner scrolling to "where are my dates?" passes a
settings panel every time — config is once-per-plan, results are the daily
read.

**Buried baselines.** The baselines panel renders at y≈4000 in a 1000px
viewport — below the heatmap, below the fever chart. Spec 001 calls the
baseline THE decision artifact; it is effectively invisible.

**Verdict under process state.** The single most valuable sentence ("5 of 7
dates at risk" / "all dates hold") is one small line among badge, rule, WIP
note, optimize hint, four buttons and comparison bars.

**Dead-end states.** "No dated initiatives — nothing to miss" reads as an
error, not an instruction. The missing-pod warning names Moose Factory and
stops. The plans list shows names only — no dates/model/baseline health.

**Three unchosen models.** WIP model, estimate model, splitting/chunking —
each defaults silently, each with a "?" tooltip, none framed as "decide
these three things once."

---

## 3. User Stories

### Story 1: Results first

**As a** returning planning manager
**I want** the plan's Order summary to be the first thing I see
**So that** my daily read starts at the answer, not at settings

### Story 2: Baselines at hand

**As a** planning manager
**I want** baseline save/compare controls in the Order header
**So that** freezing and comparing an agreed order is one click, not a scroll

### Story 3: Warnings that act

**As a** planning manager
**I want** every warning to offer its fix
**So that** I never have to diagnose the tool's own hints

### Story 4: The plan's setup as a checklist

**As a** first-time planner on a plan
**I want** one "set up this plan" card with the three model choices and
recommended defaults
**So that** finishing setup is visible progress, not three "?" discoveries

---

## 4. Acceptance Criteria

**AC 1.1: Setup collapsed, results first**

> Given a plan with roster and initiatives
> When the plan opens
> Then the Order summary area begins within the first viewport; the horizon/
> loss inputs, roster picker, upload fields, strict-deps and samples line sit
> behind a collapsed "Plan setup" disclosure, one click to expand

**AC 1.2: Setup re-opens when it must**

> Given the setup disclosure collapsed
> When the plan has NO roster or NO initiatives (the empty states)
> Then the disclosure renders expanded — the empty state IS setup

**AC 2.1: Baseline controls in the Order header**

> Given the Order view
> When rendered
> Then the baseline chip, "save as baseline" input+button and the pairwise
> compare select are reachable without scrolling past the order table; the
> full history table stays at the bottom

**AC 3.1: Actionable empty verdict**

> Given a plan with no target dates
> When the Order view renders
> Then the banner says what a date is for and how to add one (✎ editor or
> sheet column), in one sentence

**AC 3.2: Actionable missing-pod warning**

> Given initiatives referencing pods absent from the roster
> When the warning renders
> Then it offers the fix inline: "switch roster" (opens the picker) — the
> sheet-edit path stays mentioned for completeness

**AC 4.1: Setup checklist card**

> Given a plan whose WIP model or estimate model is unchosen
> When the Order view first renders
> Then a compact "Set up this plan" card lists the unchosen models with
> recommended defaults and one-click apply; once all are chosen the card
> disappears and stays gone (stored preference per plan)

**AC 4.2: The card defers to the dialog**

> Given the setup card's "why" links
> When clicked
> Then they open the ⚙ Assumptions dialog at the relevant field — the card
> routes, the dialog remains the single form

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The plan-setup row (horizon/loss, roster, uploads, strict-deps, samples) MUST render inside a collapsed `<details>`-style disclosure for complete plans, expanded for empty ones | MUST |
| FR-002 | The Order header MUST carry the baseline chip, the save-as-baseline control, and the pairwise compare entry; the history table remains below | MUST |
| FR-003 | The no-dates banner MUST name the fix (✎ editor / Target Date column) in one sentence | MUST |
| FR-004 | The missing-pod warning MUST offer "switch roster" inline, opening the roster picker | MUST |
| FR-005 | An unchosen-models checklist card MUST render above the order table until every model is chosen, with one-click "use recommended" | MUST |
| FR-006 | The plans list MUST show per-plan health chips: dated-count, estimate model, baseline count | SHOULD |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression | suites green | `go test ./...`, `node --test` |
| NFR-002 | First paint of Order summary above the fold (< 1000px) on the GWCP plan | y < 1000 | in-browser |

---

## 7. Data Model

No engine changes. The setup-card's "dismissed" state rides
`SchedulingParams` as a `setupAcknowledged` flag (PATCH /scheduling), so it
survives reloads per plan.

---

## 8. API Contract

No new endpoints.

---

## 9. Out of Scope

- The Order-view hero redesign (verdict banner scale-up, sticky headers) —
  bigger, its own pass
- Timeline visual legend — cosmetic, bundled later
- Any engine/schedule behaviour

---

## 10. Open Questions

None — all defaults confirmed with the product owner during the audit.

---

## 11. Decision Record

### Decision 1: Collapse setup behind a disclosure, not a separate page

**Context:** Config-on-top was the original layout; a "Settings" tab would
move it entirely away.

**Decision:** A same-page disclosure (expanded only for empty plans). Setup
stays one click from the results it configures, and the empty state — where
setup IS the content — shows it expanded automatically.

**Alternatives considered:**
- Separate settings tab — rejected: two-click round trip for a once-per-plan
  act; the disclosure is one.

### Decision 2: Baseline controls duplicated in the header, history stays put

**Context:** The full baselines panel (history table, compare selects) is too
wide for the header.

**Decision:** The header gets the chip + save control + compare entry; the
panel below the table keeps everything. Two entry points, one state — both
render from `current.baselines`.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Order summary first paint | below setup row + ~3000px | above the fold | in-browser y-coord |
| Baseline save reachable at | y≈4000 | y≈200 (header) | in-browser |
| Unchosen models on a fresh plan | 3 silent defaults | one checklist card | in-browser |
