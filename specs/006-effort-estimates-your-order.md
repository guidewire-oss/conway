# Effort-Based Estimates and Your-Order-First Planning

**Status:** Draft
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-25
**Story/Ticket:** discovered during GWCP planning; estimate semantics confirmed wrong in review
**Sprint/Cycle:** n/a

---

## 1. Overview

Two changes, one theme — the planner's intent leads, the engine serves it.
First, estimates become **effort** (person/pair-weeks of work) that the
scheduler divides across a pod's lanes, instead of single-lane wall-clock
weeks — the GWCP schedule is currently pessimistic by the lane factor, the
single biggest distortion in every date. Second, the Order view's default
stops being the engine's best ordering: the plan opens in **the planner's own
order** (stated priority, else sheet order), is baselined on demand, and a
prominent **Optimize** button runs the constraint-based search and presents
its proposal as a *suggestion to accept or reject* — this is a non-linear
problem with optimizations, not solutions, and the UI must never pretend
otherwise.

---

## 2. Problem

**Estimates.** "Estimate in Weeks" on the FullKit sheet reads as effort to
whoever fills it in. The scheduler today treats it as one lane's wall-clock
duration: a 60-week estimate on a 3-pair pod occupies one lane for 60 weeks
(67 after capacity loss), when the effort reading is ~22 weeks across three
lanes. Every commit week, verdict and fever point inherits that distortion.

**Ordering.** The engine runs five dispatch rules and shows its best as THE
order, with the planner's own order relegated to a reconciliation table. A
planner who has thought hard about sequence sees a stranger's order first and
must mine deltas to recover their own intent. The engine's search is
excellent *input*; as an uninvited default it reads as the tool overruling
the planner.

**Vocabulary.** The levers (drum, buffer, WIP models, stagger, full-kit)
are constraint-management jargon. The glossary has 15 entries; the levers
deserve the same affordance everywhere they appear.

---

## 3. User Stories

### Story 1: Estimates are effort, divided by lanes

**As a** planning manager
**I want** a pod's estimate to mean total effort, shared across its lanes
**So that** a 60-week estimate on a 3-lane pod takes ~22 weeks, not 67

### Story 2: The plan opens in MY order

**As a** planning manager
**I want** the Order view to show my stated priority (or the sheet's order
when no priorities are set) as the working plan
**So that** the starting point is what I intended, not what the engine prefers

### Story 3: Optimize is an offer, not a takeover

**As a** planning manager
**I want** one button that runs the constraint-based search and shows its
best ordering side-by-side with mine, priced
**So that** I can accept it, take parts of it, or keep my order — and the
system remembers which I chose

### Story 4: Every lever explains itself

**As a** planner new to constraint management
**I want** a glossary tooltip on every scheduling lever and every column
**So that** "drum target utilization" is one hover away from plain language

---

## 4. Acceptance Criteria

### Story 1: effort ÷ lanes

**AC 1.1: The duration divides by lanes and rounds up**

> Given a pod with 3 lanes and a 60-week estimate at 10% capacity loss
> When the schedule is computed
> Then the slice duration is ceil(60 ÷ 3 ÷ 0.9) = 23 weeks

**AC 1.2: The slice occupies the lanes it needs**

> Given a slice of 23 weeks on a 3-lane pod
> When placed
> Then it occupies min(lanes-needed, tracks) lanes each week — a pod's
> parallelism accelerates its own work, not only other initiatives'

**AC 1.3: The estimate model is labelled**

> Given a plan
> When the Order view renders
> Then it states which estimate model is in force (effort ÷ lanes vs
> wall-clock), because the same number means different things

**AC 1.4: Wall-clock stays available**

> Given a plan whose estimates were entered as single-lane durations
> When the planner chooses the wall-clock model
> Then schedules match today's behaviour exactly (migration safety)

### Story 2: your order first

**AC 2.1: Default order is the planner's**

> Given a plan with stated priorities
> When the Order view opens
> Then rows follow stated priority; with none set, the sheet's row order;
> the engine's ranking is shown as a *suggested* column, not the spine

**AC 2.2: The stated-order schedule is priced**

> Given the planner's order
> When rendered
> Then its objective score and date verdicts are computed and shown — the
> planner's order is a first-class schedule, not an error state

### Story 3: optimize as an offer

**AC 3.1: One button, one proposal**

> Given the planner's order on screen
> When the planner activates "Optimize order"
> Then the engine's best ordering is presented beside the planner's: both
> objective scores, per-initiative deltas, and which rule won

**AC 3.2: Accept is explicit and remembered**

> Given a proposal the planner accepts
> When the order is recomputed
> Then the engine's order becomes the working order and the plan records the
> choice; rejecting keeps the planner's order, equally recorded

**AC 3.3: No priorities given, sheet order is the priority**

> Given no stated priorities
> When the plan is saved or baselined
> Then sheet order IS the stated order for ordering purposes, and the
> baseline freezes it (the user's rule, restated)

### Story 4: tooltips everywhere

**AC 4.1: Every lever carries a glossary affordance**

> Given the assumptions dialog and the Order header
> When rendered
> Then every lever and metric has the `?` tooltip with plain language first
> and theory credit after (the existing glossary pattern)

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The estimate model MUST be selectable per plan: `effort` (default for new plans) divides each pod estimate by the pod's lanes; `wall-clock` preserves current semantics | MUST |
| FR-002 | Under `effort`, slice duration MUST be ceil(weeks ÷ lanes ÷ (1 − capacityLoss)), lanes = EffectiveTracks, minimum 1 week | MUST |
| FR-003 | Under `effort`, a slice MUST occupy min(ceil(totalLaneWeeks), tracks) lanes per week, so concurrent capacity at the pod is consumed by the work itself | MUST |
| FR-004 | The Order view MUST default to stated priority order, else sheet order; the engine's proposal renders as a suggestion with its own price | MUST |
| FR-005 | A single "Optimize order" action MUST run the multi-rule search and present both orders side by side with scores and deltas | MUST |
| FR-006 | Accepting or rejecting a proposal MUST be explicit, persisted on the plan, and visible ("your order" / "engine's order (accepted 12 Jan)") | MUST |
| FR-007 | With no stated priorities, sheet order MUST be treated as the stated order for ordering and baseline purposes | MUST |
| FR-008 | Every lever, metric and column in the assumptions dialog, Order header and table MUST carry a glossary tooltip; new entries: lanes, effort estimate, objective, optimize, estimate model | MUST |
| FR-009 | The existing dispatch-rule search, WIP models, drum stagger and calendar rules continue to apply unchanged on top of either estimate model | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression on wall-clock plans | suites green, fixture unchanged semantics | `go test ./...` |
| NFR-002 | Scheduling 35 initiatives × 35 pods under effort model | < 2s (parity with today) | `go run ./tools/schedbench` |
| NFR-003 | The Order view's first paint needs no second round-trip | one schedule call | in-browser |

---

## 7. Data Model

- `SchedulingParams.EstimateModel`: `effort` \| `wall-clock` (absent = existing
  plans keep wall-clock — the migration-safe default)
- `WorkSlice.LanesUsed`: how many tracks the slice occupies per week
- `PlanRow.AcceptedOrdering`: `stated` \| `engine`, with a timestamp, once a
  proposal is accepted or rejected

---

## 8. API Contract

No new endpoints. `/schedule` gains `estimateModel` semantics (param, not
payload); plan PATCH stores the accepted-ordering marker.

---

## 9. Out of Scope

- Solving for the optimal order (this is NP-hard; the multi-rule search plus
  the planner's judgement IS the method — Decision 1)
- Splitting one pod's estimate across *time-phased* lane counts (lane count
  is fixed for the period; calendars already reduce capacity)
- Actuals (specs/001 Story 10)

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should accepting an engine proposal auto-save a baseline, or leave baselining explicit? | Anoop | 2026-08-25 | **Resolved 2026-08-25 — explicit, and ask.** Accepting a proposal changes the working order only; the completion of the accept flow offers to save a baseline (one click, pre-filled name, dismissible). |
| Q2 | In the table, is the engine's suggested rank a column beside stated rank (current reconciliation pattern) or a toggle between two full views? | Anoop | 2026-08-25 | **Resolved 2026-08-25 — column, one view.** The engine's suggested rank sits beside the stated rank with the move arrow, deltas visible without switching views. |

---

## 11. Decision Record

### Decision 1: Optimization is a search presented as a suggestion, never a solution

**Context:** Sequencing under capacity, calendars, dependencies and WIP gates
is non-linear; no algorithm returns "the" optimal order. The current UI
nevertheless opened with the engine's best run as if it were the answer.

**Decision:** The planner's stated order (or sheet order) is the working
plan. The engine's multi-rule search runs on demand and is presented as a
priced suggestion the planner accepts, partially adopts (pins exist for
exactly this) or rejects. Both outcomes are recorded.

**Alternatives considered:**
- Keep engine-first with reconciliation — rejected: it reverses who serves
  whom; the planner mines deltas to recover their own intent.
- Exhaustive/metaheuristic search for better orderings — rejected for now:
  the five dispatch rules already give good coverage, and presenting ONE
  confident answer to a non-linear problem is the failure mode being fixed.

**Consequences:** The stated-order schedule must be computed and priced on
every render (it already is — `StatedOrderObjectiveScore`). The
reconciliation table inverts from "what the engine did to you" to "what the
engine suggests".

### Decision 2: Effort ÷ lanes, with wall-clock as the migration-safe fallback

**Context:** Estimates on real sheets mean effort. Dividing by lanes is the
correct reading, but every existing plan was scheduled under wall-clock
semantics.

**Decision:** New plans default to `effort`; existing plans carry
`wall-clock` until changed; the model in force is labelled in the UI.

**Alternatives considered:**
- Effort-only, migrate everyone — rejected: silently halving every existing
  commit week is exactly the unexplained-number failure spec 001 bans.

**Consequences:** A slice occupies multiple lanes, so pod concurrency for
OTHER initiatives drops while it runs — this is honest (the pod really is
busy) and the heatmap already renders multi-track occupancy.

### Decision 3: Sheet order is stated order when no priorities are given

**Context:** The user's rule: absent priorities, the order the initiatives
appear in the sheet IS the priority.

**Decision:** rankOrder's `stated-priority` rule falls back to sheet index
(it already does for unranked rows); the Order view's default sort does the
same, and baselines freeze whatever order was in force.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| GWCP longest chain (52w period) | week 79+ | materially inside, honestly beyond-horizon | schedbench + in-browser |
| First thing a planner sees | engine's order | their own order, priced | in-browser |
| Glossary entries on levers | 15 general | +5 lever-specific | term() call audit |
