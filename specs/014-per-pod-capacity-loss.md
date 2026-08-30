# Per-Pod Capacity Loss Override

**Status:** Draft
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-30
**Story/Ticket:** high-value polish, user-directed; semantics confirmed by chat (per-pod loss override)
**Sprint/Cycle:** n/a

---

## 1. Overview

Let each pod carry its own capacity-loss percentage — the fraction of its
tracked time that never becomes product work (ops burden, support, on-call,
ramp) — instead of every pod inheriting the plan's single global figure. An
ops-heavy pod at 30% and a product pod at 5% plan honestly, where today both
get the plan's 10% and the schedule quietly lies to one of them.

---

## 2. Problem

Capacity loss is the crudest number in the model, and it is applied
uniformly. The plan header offers one global "capacity loss %" — but pods
measurably differ: a pod that owns production on-call loses far more of its
tracks than a greenfield product pod. With one number, the manager either
inflates the global loss (penalizing every pod) or keeps it low (and the
ops-heavy pod's commitments are systematically optimistic).

Concrete example: two pods with 5 tracks each and a 10% global loss are
modeled at 4.5 effective track-weeks a week. If the first really loses 30%
(3.5 effective) and the second 2% (4.9), the plan over-commits the first by
a week a week and under-commits the second — invisible until the dates slip.

---

## 3. User Stories

### Story 1: Model what each pod really delivers

**As a** planning manager
**I want** to set a capacity-loss percentage per pod, inheriting the plan's
global figure where I don't
**So that** the schedule reflects each pod's real delivery rate.

### Story 2: Enter it where the roster is entered

**As a** planning manager
**I want** the loss override to ride the roster upload as a column
**So that** it is part of the roster I already maintain, not a second form.

### Story 3: See which pods carry an override

**As a** planning manager
**I want** the pod's effective loss visible on its sheet and timeline
**So that** an unusual override is legible, not a hidden constant.

---

## 4. Acceptance Criteria

### Story 1: Per-pod honesty

**AC 1.1: An override changes only that pod's arithmetic**

> Given a plan with a 10% global loss and one pod overridden to 30%
> When the schedule computes
> Then that pod's slice durations and weekly capacity use 30%
> And every other pod still uses 10%.

**AC 1.2: No override means inherit**

> Given a pod with no override set
> When the schedule computes
> Then the pod uses the plan's global loss exactly as today.

**AC 1.3: Utilization and scheduling agree**

> Given an overridden pod
> When ρ is computed for the pod load and the timeline renders
> Then both use the same effective loss
> And the pod's ρ cannot disagree with the durations on its tracks.

### Story 2: Roster-borne input

**AC 2.1: The upload column parses**

> Given a roster with a "Capacity Loss %" column
> When the roster uploads
> Then each pod's override is read from its cell
> And "15", "15%", and "0.15" all read as 15%.

**AC 2.2: Out-of-range values are refused with the pod named**

> Given a cell of "150%" or "-3"
> When the roster uploads
> Then the upload is rejected naming the pod and the value
> And no roster is saved.

### Story 3: Legible overrides

**AC 3.1: The override is visible where the pod is visible**

> Given a pod with a loss override
> When the pod's sheet or timeline card renders
> Then the effective loss percentage is shown
> And pods inheriting the global figure say so.

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The roster parser MUST accept an optional "Capacity Loss %" column (also matching "loss") | MUST |
| FR-002 | A pod's override MUST apply to slice durations, placement capacity, and pod ρ — one definition of effective loss used everywhere | MUST |
| FR-003 | A pod without an override MUST inherit the plan's global capacity loss | MUST |
| FR-004 | Values MUST satisfy 0 ≤ override < 1; inputs above 1 are read as percent; out-of-range uploads are refused naming the pod | MUST |
| FR-005 | The pod sheet and by-pod timeline MUST show the pod's effective loss | SHOULD |
| FR-006 | The portfolio fit line MUST sum per-pod effective capacity, not a global average | SHOULD |
| FR-007 | The sample teams CSV SHOULD carry the column with empty cells (all inherit) | MAY |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | Baselines capture overrides with no new machinery | rosters are frozen wholesale | existing baseline save |
| NFR-002 | Engine determinism unchanged | same inputs, same schedule | existing Ginkgo suite |

---

## 7. Data Model

**Team** gains:
- `capacityLoss`: float64 — the pod's override (0 = inherit the plan global)

Derived:
- `effectiveLoss(team, global)` = team override when > 0, else the plan global

### Relationships
- A plan has many teams; a baseline freezes the teams (override included) —
  no baseline change needed.

---

## 8. API Contract

No new endpoints. The roster upload (`POST /api/plan/{id}/teams` and the
roster attach) carries the new column; the schedule/simulate responses gain
no fields (the override is an input, not an output).

---

## 9. Out of Scope

- Editing overrides in-app (re-upload is the path; an inline roster editor is
  a later slice)
- Per-pod loss history or audit
- Multipliers on tracks or effort (rejected — see Decision 1)

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should the plan header's global loss stay, or become "set all pods"? | Anoop | 2026-09-05 | pending — default: stays; it is the inherited default, and "set all" invites accidental stomps |

---

## 11. Decision Record

### Decision 1: The factor is a per-pod LOSS override, not a track or effort multiplier

**Context:** Pods differ in how much of their tracked time becomes product
work. Three semantics were on the table: a loss override, a fractional track
multiplier, and an effort multiplier.

**Decision:** The override is a per-pod capacity loss. Effective weekly
capacity = tracks × (1 − effectiveLoss). A pod without an override inherits
the plan's global figure.

**Alternatives considered:**
- Track multiplier — rejected: redundant with the existing whole-number
  Tracks override, and fractional tracks promise precision the weekly model
  cannot keep.
- Effort multiplier — rejected: it rewrites what the planner's estimate
  means; the estimate stays "work in weeks", the loss is the pod's honesty
  about its own time.

**Consequences:** Every site that consumes capacity loss must use the same
per-pod effective value — durations (sliceWeeks), placement (splitPlace),
pod ρ (PodLoad), and the portfolio fit line — or the views disagree. The
global loss becomes a default, not a mandate.

### Decision 2: The override rides the roster, not a new form

**Context:** Pods are already maintained as a roster (upload or attached
roster); a second editing surface would drift from it.

**Decision:** The override is a roster column. Rosters freeze into baselines
wholesale, so baselines capture overrides with no new machinery.

**Alternatives considered:**
- An in-app roster editor — deferred: real value, separate slice.

**Consequences:** Managers maintain one artifact; the sample CSV ships the
column with empty cells so the shape is discoverable.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Pods whose real delivery rate differs from the global figure | modeled as identical | independently settable | in-browser |
| Estimate calibration for ops-heavy pods | systematically optimistic | variance reported by spec 001 Story 10 | once actuals land |
