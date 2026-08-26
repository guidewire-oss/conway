# Drag-to-Edit the Timeline, Fully Propagated

**Status:** Draft
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-26
**Story/Ticket:** next slice after spec 007; user-directed
**Sprint/Cycle:** n/a

---

## 1. Overview

Let the planner reshape the plan with the mouse: drag a slice on the Gantt
timeline (move it, stretch it) and every derived number follows — start week,
finish, commit, verdicts, fever point, heatmap, pod loads, the Order table,
both timeline lenses. The edit round-trips through the same server model so
pod view and initiative view can never disagree, and the result saves as a
baseline comparable against any other baseline (already built, spec 005).

---

## 2. Problem

Every schedule change today is indirect: edit an assumption, re-upload, or
PATCH an initiative field, then re-read the chart. The most natural planning
gesture — "what if this started two weeks later?" — has no affordance. And
because the timeline is a *render* of the schedule, naive client-side dragging
would desynchronize the three views (initiative lens, pod lens, Order) unless
the edit goes through the engine.

---

## 3. User Stories

### Story 1: Drag a slice to move it

**As a** planning manager
**I want** to grab a bar on the timeline and slide it left or right
**So that** I can explore "what if it started later" without forms

### Story 2: Stretch a slice's span

**As a** planning manager
**I want** to drag a bar's edge to lengthen or shorten the work
**So that** a re-estimate is one gesture

### Story 3: Every view agrees immediately

**As a** planning manager
**I want** the drag to recompute the whole schedule — Order, both timeline
lenses, heatmap, fever chart, verdicts
**So that** there is never a stale or contradictory view after an edit

### Story 4: Save and compare the hand-shaped plan

**As a** planning manager
**I want** to baseline the dragged plan and compare it against another
baseline
**So that** the hand-shaped alternative is a first-class scenario

---

## 4. Acceptance Criteria

**AC 1.1: A drag moves a slice in whole weeks**

> Given a scheduled slice on either timeline lens
> When the planner drags it horizontally and releases
> Then the slice's start (and finish) move by the dragged whole weeks, and the
> schedule recomputes with the slice pinned to that start

**AC 1.2: An edge drag resizes**

> Given a slice's left or right edge
> When the planner drags the edge
> Then the span grows or shrinks in whole weeks and the schedule recomputes
> with that duration for the slice

**AC 1.3: Constraints are honoured or refused visibly**

> Given a drag that would start inside a freeze, overlap a full pod, or break
> a dependency
> When released
> Then the engine either snaps to the nearest legal placement or refuses with
> a named reason — never a silently-illegal chart

**AC 1.4: Both lenses and the Order update together**

> Given a successful drag in the pod lens
> When the view re-renders
> Then the initiative lens, Order table (ranks, weeks, verdicts), heatmap,
> fever chart and WIP-model prices all reflect the new schedule; no view
> serves stale numbers

**AC 1.5: Baseline round-trip**

> Given a dragged plan
> When the planner saves it as a baseline and compares it against a prior one
> Then the pairwise compare (spec 005) reports the dragged deltas

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | Slices on both timeline lenses MUST be draggable (move) and edge-resizable (stretch) with whole-week snapping | MUST |
| FR-002 | A release MUST send a pin (initiative + pod + pinned start week) or a duration override to the server, not a client-side redraw | MUST |
| FR-003 | The server MUST recompute the schedule from the edited pins/overrides with all constraints (WIP, calendars, dependencies, stagger) in force, snapping or refusing illegal placements with a named reason | MUST |
| FR-004 | The recomputed schedule MUST replace every dependent view: Order table, both lenses, heatmap, fever chart, reconciliation, fit report | MUST |
| FR-005 | Edited pins/overrides MUST persist on the plan (PATCH), so a reload keeps the hand-shaped plan; a "clear pins" affordance removes them | MUST |
| FR-006 | Undo of the last drag (one level, ⌘Z / button) MUST be offered until the next save | SHOULD |
| FR-007 | Dragging MUST be disabled on read-only/draft states and touch devices without pointer precision | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression | suites green | `go test ./...`, `node --test` |
| NFR-002 | Drag-to-recompute on 29×35 the reference plan | < 1.5s | in-browser |
| NFR-003 | Draggable affordance discoverable | cursor + hint text on first drag | in-browser |

---

## 7. Data Model

- `Initiative.PinnedStarts map[string]int` — pod → pinned start week for that
  initiative's slice (the drag's persisted form)
- `InitiativeEdit.PinnedStarts *map[string]int` — the PATCH shape (pointer so
  "clear all pins" is expressible)
- Duration overrides ride the existing estimate edit (the ✎ dialog's per-pod
  weeks), which a stretch-drag maps onto

---

## 8. API Contract

No new endpoints. `POST /schedule` accepts the stored pins (it already accepts
levers); `PATCH /initiatives` persists them.

---

## 9. Out of Scope

- Free-form sub-week dragging (whole weeks only — the model is weekly)
- Cross-pod dragging (moving work to a different team is a roster/estimate
  change, not a schedule gesture)
- Multi-select group drags
- Undo history beyond one level

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should a drag-pin expire when the underlying estimate changes (stale pin warning), or persist until cleared? | Anoop | 2026-08-30 | pending — default: persist, warn when the pinned start predates a re-estimate |
| Q2 | Edge-stretch on the LEFT edge changes start, on the RIGHT changes finish — should the other end stay fixed (anchor), or should start stay and duration grow? | Anoop | 2026-08-30 | pending — default: the dragged edge moves, the opposite edge anchors |

---

## 11. Decision Record

### Decision 1: Drags are pins re-run through the engine, not client geometry

**Context:** The tempting implementation redraws the bar where the mouse
released. That desynchronizes: the pod lens, initiative lens and Order would
each need their own patch, and every constraint (capacity, calendars,
dependencies) would need client-side reimplementation.

**Decision:** A released drag persists a **pin** (initiative, pod, start week)
and the server recomputes the entire schedule with the pin in force. All views
render the one recomputed schedule. Illegal pins snap to the nearest legal
week with the reason named.

**Alternatives considered:**
- Client-side geometry with per-view patching — rejected: three sources of
  truth, constraints duplicated, drift guaranteed.

**Consequences:** Every drag is a schedule round-trip (fast: one engine run).
The chart after a drag is engine-legal by construction — the planner can never
draw a schedule the constraints refuse.

### Decision 3: Lane pins are positional, overlap is refused loudly

**Context:** Vertical dragging needs a target: "track 3 of this pod". Tracks are
engine-derived, so a lane pin must pin the SLICE'S POSITION IN THE POD'S LANE
STACK, not a globally-stable track id.

**Decision:** `PinnedLanes` records the pod-relative lane offset (first lane the
slice occupies). The engine attempts that offset; if another slice already
holds those lanes in any overlapping week, the drop is REFUSED with a named
overlap error — never silently re-lane-packed. Horizontal movement remains the
start-week pin. Dragging is offered ONLY in the by-pod lens (Decision 2), and
the lens gains a fullscreen mode (ESC to exit) because lane-accurate dragging
needs the space.

**Alternatives considered:**
- Silently snapping to a free lane — rejected: the user's explicit ask is an
  overlap error; quiet snapping is how "the chart ignored me" reads.

**Consequences:** Lane pins make the pod's lane stack partly hand-shaped; the
packing walk honours pinned offsets before packing the rest.

### Decision 2: Baselines capture the hand-shaped plan as-is

**Context:** Spec 005 already compares two stored schedules; nothing about a
dragged plan is different once recomputed.

**Decision:** Save-as-baseline and pairwise compare work unchanged on dragged
plans. No new machinery.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Gestures to test "start 2w later" | ~6 (dialog + save + recompute + navigate ×2) | 1 drag | in-browser |
| Views in agreement after edit | manual to verify | guaranteed | AC 1.4 |
