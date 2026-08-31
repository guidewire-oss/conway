# Baselines Drawer

**Status:** Draft
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-30
**Story/Ticket:** user-directed UX consolidation ("the experience is fragmented on saving and viewing baselines")
**Sprint/Cycle:** n/a

---

## 1. Overview

Baselines get one home: a slide-over drawer, reachable from the status chip in
one click from any view, that holds everything — saving the current order
under a name, the history with activate/compare/delete, and the pairwise
comparison result. The status chip stays; the buried panel, the duplicated
save entries, and the save modal all collapse into it.

---

## 2. Problem

After the save-modal fix, the baseline concept is still spread across four
surfaces: a chip (status), a header button (save entry), a panel button
(save entry), and a history panel buried below a 29-row order table, a
heatmap, and an assumptions card. Viewing or activating a baseline costs
scrolling past three screens of the thing it manages; the chip's "▾" implies
a dropdown but scrolls; divergence ("inputs have moved") names no next step;
and a mistakenly named baseline cannot be removed — the list only grows.

Concrete example: a manager reviews the chip, sees "agreed: Kickoff — inputs
have moved", and wants to freeze the changed plan as a new baseline. The
affordance for that decision lives four screens down, behind a table.

---

## 3. User Stories

### Story 1: Manage baselines in one place

**As a** planning manager
**I want** one drawer that holds saving, history, activation, and comparison
**So that** I never have to remember which of three doors does what.

### Story 2: Decide in context

**As a** planning manager
**I want** the drawer to slide over the Order view, leaving it visible
**So that** activation and comparison decisions are made while looking at the
order they are about to freeze or restore.

### Story 3: Correct mistakes

**As a** planning manager
**I want** to delete a baseline I saved by mistake
**So that** the history stays a trustworthy record of real agreements.

---

## 4. Acceptance Criteria

### Story 1: One home

**AC 1.1: The chip opens the drawer**

> Given any plan view
> When the status chip is clicked
> Then the baselines drawer opens — regardless of chip state or current view.

**AC 1.2: Everything lives in the drawer**

> Given the drawer is open
> Then it shows the save row, the full history with per-baseline actions,
> and the comparison selects and result — nothing baseline-related renders
> anywhere else.

### Story 2: Decide in context

**AC 2.1: The Order view stays visible**

> Given the drawer is open over the Order view
> When the planner activates or compares baselines
> Then the order table remains visible beside the drawer
> And the drawer never covers more than half the viewport.

**AC 2.2: A re-render does not strand the drawer**

> Given the drawer is open
> When the Order view re-renders (activation, lever change, drag)
> Then the drawer stays open and its list refreshes.

### Story 3: Correctable history

**AC 3.1: Delete with confirmation**

> Given a baseline that is not in a pending delete-confirm state
> When Delete is clicked
> Then the button asks for confirmation in place
> And a second click deletes it and the chip updates.

**AC 3.2: Divergence offers the next step**

> Given the active baseline's inputs have moved
> When the drawer opens
> Then the save row is highlighted with "the inputs have moved — save this
> changed plan as a new baseline".

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The status chip MUST open the drawer in one click from any view | MUST |
| FR-002 | The drawer MUST hold the save row (inline name field), the history (saved date, divergence tag, Activate, Compare, Delete), and the compare selects and result | MUST |
| FR-003 | Saving MUST require a name, freeze the stored inputs (draft preview refused), and refresh the drawer and the chip in place | MUST |
| FR-004 | Delete MUST be a two-step in-place confirmation and MUST be refused for the request that would leave the plan inconsistent; deleting the active baseline leaves the plan with none | MUST |
| FR-005 | A diverged active baseline MUST highlight the save row with a named next-step message | SHOULD |
| FR-006 | The Order header's save button and the bottom baseline panel MUST be removed (one home) | MUST |
| FR-007 | The drawer MUST close on ESC and on a backdrop click | SHOULD |
| FR-008 | The health report's baseline line MUST keep naming the active baseline | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | Drawer width ≤ half the viewport | ≤ 720px on a 1440px screen | in-browser |
| NFR-002 | List refresh after save/activate/delete | no full page reload | network log |

---

## 7. Data Model

No new entities. The drawer reads `current.baselines` (id, name, active,
createdAt, diverged) and `current.baselineCompare`; the server gains a DELETE
route for a baseline row.

---

## 8. API Contract

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| DELETE | /api/plan/{id}/baseline/{bid} | delete one baseline (scoped by plan) | — | {"ok": true} |

Existing save/activate/compare endpoints unchanged.

---

## 9. Out of Scope

- Renaming saved baselines (immutable snapshots; a rename endpoint exists
  server-side but no UI is offered yet)
- Baseline diff beyond the existing pairwise compare
- Multi-select or bulk delete

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should deleting the ACTIVE baseline be refused with a message, or allowed (leaving the plan with none)? | Anoop | 2026-09-05 | resolved 2026-08-30: allowed, and the newest remaining baseline becomes active — the chip reports the plan's latest agreement, never a surprising "none" while others exist |

---

## 11. Decision Record

### Decision 1: A drawer over the working view, not a modal and not a tab

**Context:** Three surfaces were considered: a save modal (too cramped for a
list — the previous fix), a dedicated Baselines tab (implies a working view
you live in, when baselines are an artifact manager), and a slide-over drawer
over the Order view.

**Decision:** The drawer slides over the Order view, keeping it visible.
Activation and comparison are decisions made *about* the order, so the order
stays in sight. One home holds everything baseline-related.

**Alternatives considered:**
- Save modal (the interim fix) — rejected: it solved saving but left history,
  activation and comparison buried below the order table.
- A fourth view tab — rejected: wrong mental model; baselines are not a peer
  working surface, and the compare context is the order itself.

**Consequences:** The chip becomes a true control (one click, any state, any
view). The Order header and the bottom panel lose their baseline controls.

### Decision 2: Delete is in-place two-step, never a browser confirm

**Context:** Destructive actions in the app avoid `confirm()` dialogs (they
are unstyled, untestable, and block the event loop); the panel pattern is an
in-place state change.

**Decision:** Delete renders as "Delete"; clicking it turns the row's button
into "Confirm delete?" for that row only; a second click deletes; any other
click or a re-render resets it.

**Consequences:** No lost baselines from a stray click, no browser dialogs,
and the confirm state is testable markup.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Surfaces holding baseline controls | 4 (chip, header, panel, modal) | 2 (chip, drawer) | markup audit |
| Clicks to activate a baseline from the Order view | scroll + click | 2 (chip, activate) | in-browser |
| Accidental-baseline cleanup | impossible | delete with confirm | in-browser |
