# Plan — Execution Order for a Future Period

**Status:** Draft
**Author(s):** Anoop Gopalakrishnan (with Claude)
**Date:** 2026-08-17
**Story/Ticket:** _TBD — create before implementation starts_
**Sprint/Cycle:** —

---

## 1. Overview

Plan today answers one question: *if we attempted everything in this period at
once, who would choke?* It cannot answer the question a planner actually has to
settle: **in what order do we start these initiatives, and do the ones carrying
dates survive that order?**

This feature adds an execution-order engine to the Plan lens. Initiatives gain a
stated priority, a target date, a commitment tier and a cost of delay. Conway
lays them out across the weeks of the period against finite pod capacity,
cross-pod dependencies, named leads, calendars and a work-in-progress limit —
then proposes an order. Where its proposal departs from the planner's stated
priority, it must show the trade in the planner's own terms ("this cannot hit
15 Nov below priority 3; raising it pushes *Telemetry GA* out four weeks"), so a
human makes the call. Priority the planner marks non-negotiable is a constraint
the engine routes around, never an argument it wins.

---

## 2. Problem

A planner uploads a roster and a FullKit matrix. Conway reports per-pod
utilization and a dependency network. On the shipped demo plan that reads:
Delta carries 49 weeks of demand against 46.8 weeks of capacity (ρ 1.05, red);
Ember carries 46 (ρ 0.98, hot amber).

That is a true and useful diagnosis, and then the conversation stops, because
the next five questions have no home in the tool:

1. Delta is in the path of five initiatives. **Which one does Delta start
   first?** Conway models all five as running for the whole period.
2. *Telemetry GA* is committed to a customer for week 20. **Does it land?**
   There is no target date in the model and no start week to measure from.
3. The planner believes *Managed database MVP* is priority 2. **Is that
   consistent with its date, or is it quietly the reason something else is
   late?** Priority does not exist in the model at all.
4. Three initiatives need the same architect in their first month. Nothing in
   the tool knows that, though the sheet already names the leads.
5. When a date will not fit, **what is the cheapest way to rescue it** — descope,
   add tracks, move capacity from a pod with slack, or move the date?

What happens without it: order is settled in a spreadsheet by whoever argues
best, dates are committed without any feasibility check, the constraint pod
receives work in arrival order rather than value order, and the collision is
discovered in month four when the buffer is already gone. The org's own
retrospective language for this is in `flow-rules-notes.md`: *"too many
priorities means no priority"*, and *"these started, the clock ran, the work
never began"*.

The pain is felt by the planning manager who owns the period, the PM who sold a
date, and the pod lead who discovers in month four that they are on five
critical paths at once.

---

## 3. User Stories

### Story 1: Propose an execution order

**As a** planning manager for a future period
**I want** Conway to propose the order in which initiatives should start, and when each will finish
**So that** the sequence is derived from capacity and commitments rather than from the loudest argument

### Story 2: Test a committed date

**As a** PM or program manager
**I want** to enter an initiative's target date and see whether the proposed order lands it
**So that** I find out before I commit, not after the buffer is burned

### Story 3: Protect a non-negotiable priority

**As an** exec sponsor
**I want** to mark specific initiatives as fixed priority or fixed date
**So that** the engine schedules around my commitment instead of proposing that I relax it

### Story 4: See when each pod is loaded

**As a** pod lead
**I want** to see my pod's week-by-week load under the proposed order, and which initiatives sit in my queue
**So that** I can see the months where I am over-committed while there is still time to change them

### Story 5: Rescue a date that will not fit

**As a** planning manager
**I want** ranked, priced options when a date misses — raise its priority, descope, add or move capacity, or move the date
**So that** I choose a remedy with its cost visible rather than guessing

### Story 6: Compare my order with the proposed one

**As a** leadership audience
**I want** to see what my own stated priority order costs against the engine's proposal
**So that** the disagreement is about a number rather than about opinions

---

## 4. Acceptance Criteria

### Story 1: Propose an execution order

**AC 1.1: An order is produced from a plan that has no new attributes**

> Given a plan with a roster and initiatives and no priorities, dates, tiers or costs of delay
> When the planner opens Execution order
> Then every initiative receives a proposed rank, a start week and a finish week
> And the order is derived from dependency structure and consumption of the constraint pods
> And no existing Plan view changes its numbers

**AC 1.2: Finite capacity is respected**

> Given a pod with 2 tracks that appears in five initiatives
> When the order is computed
> Then in no week of the period are more than 2 of that pod's work slices in progress
> And the remaining slices show a start week later than the period start

**AC 1.3: Dependencies and cross-site handoffs are respected**

> Given an initiative where pod B's work depends on pod A's, and A and B are at sites with no working-hours overlap
> When the order is computed
> Then B's slice starts no earlier than A's finish plus the cross-site handoff latency
> And the handoff delay is attributed to the seam, visible in the initiative's breakdown

**AC 1.4: The result is deterministic**

> Given the same plan and the same scheduling parameters
> When the order is computed twice
> Then both results are identical, field for field

**AC 1.5: Work in progress is capped at release**

> Given an org WIP limit of 4 concurrent initiatives
> When the order is computed
> Then no week has more than 4 initiatives in flight
> And initiatives held back name "org WIP limit" as the reason for their start week

### Story 2: Test a committed date

**AC 2.1: A date that fits is confirmed**

> Given an initiative with a target date in week 20 that the proposed order finishes in week 14 with a buffered commit week of 17
> When the planner views it
> Then it is marked on time
> And both the raw finish week and the buffered commit week are shown

**AC 2.2: A date that misses is quantified and explained**

> Given an initiative with a target date in week 12 that the proposed order commits in week 19
> When the planner views it
> Then it is marked late by 7 weeks
> And the binding constraint is named (the pod, lead, dependency or release rule that set its start)

**AC 2.3: Structural infeasibility is distinguished from contention**

> Given an initiative whose own dependency chain is 30 weeks long and whose target date is in week 12
> When the order is computed
> Then it is reported as structurally infeasible — no ordering can meet the date
> And it is listed separately from initiatives that miss only because of contention for capacity

**AC 2.4: A date in the past or outside the horizon is rejected at entry**

> Given a planner entering a target date before the period start or after the horizon
> When they save it
> Then the value is rejected with a message naming the period's start and end dates
> And no schedule is recomputed

**AC 2.5: An initiative with unestimated work is flagged, not silently scheduled**

> Given an initiative where one in-path pod has no weeks estimate ("TBD")
> When the order is computed
> Then the initiative is scheduled using only its estimated work
> And it carries a low-confidence marker naming the unestimated pods
> And any date verdict on it is marked provisional

### Story 3: Protect a non-negotiable priority

**AC 3.1: A locked priority is never lowered**

> Given an initiative at stated priority 2 marked as fixed priority
> When the order is computed
> Then its proposed rank equals its stated rank relative to all other initiatives
> And whenever it competes with a worse-priority initiative for the same pod track or lead, it is dispatched first

**AC 3.2: A locked date constrains rather than argues**

> Given an initiative marked as a fixed date commitment
> When the order is computed
> Then the engine treats missing that date as the most expensive outcome available
> And if it still cannot be met, it is reported as a commitment at risk with the specific constraint that breaks it

**AC 3.3: Two locks that cannot both hold are surfaced, not silently broken**

> Given two initiatives with fixed dates that both require the same 2-track pod for overlapping spans
> When the order is computed
> Then both are reported as a conflicting-commitments pair
> And at least one relaxation is offered for each (unlock, descope, add capacity, move capacity, move the date)
> And neither lock is silently violated

### Story 4: See when each pod is loaded

**AC 4.1: Week-by-week load per pod**

> Given a computed order
> When the planner opens the pod view
> Then each pod shows its utilization for every week of the period, and which initiatives occupy it in each week
> And weeks at or above full capacity are marked red, at or above 0.85 amber, matching the app's existing thresholds

**AC 4.2: Period-level utilization stays consistent with the existing model**

> Given a plan with no idle enforced by WIP limits, calendars or dependencies
> When the order is computed
> Then the mean of a pod's weekly utilization equals its existing flat ρ within 2 percentage points
> And where it does not, the difference is attributable to reported idle time

**AC 4.3: A pod's queue is visible in order**

> Given a pod appearing in five initiatives
> When the planner clicks the pod
> Then its slices are listed in scheduled start order with wait time before each

### Story 5: Rescue a date that will not fit

**AC 5.1: Options are proposed and priced**

> Given an initiative that misses its target date by 7 weeks
> When the planner asks for options
> Then the engine returns candidate remedies, each with the resulting date verdict and its cost expressed as what it does to the rest of the portfolio
> And the list is ordered cheapest-first by weighted lateness across all initiatives

**AC 5.2: Raising priority is offered as an explicit remedy with its victims named**

> Given a late initiative that would land if it started earlier
> When the engine proposes raising its priority
> Then the proposal states the priority it would need
> And names each initiative pushed out by that change and by how many weeks

**AC 5.3: Capacity transfer is offered only where it is plausible**

> Given a binding pod at a site with a donor pod that has slack
> When the engine proposes moving tracks from the donor
> Then the proposal carries a plausibility rating derived from working-hours overlap between the two sites and their prior collaboration in this plan
> And the transferred capacity delivers at reduced effectiveness for a ramp period
> And the donor loses that capacity immediately

**AC 5.4: A transfer that would break the donor is not proposed**

> Given a candidate donor pod whose own utilization would exceed 0.85 in the transfer window, or which has a fixed-date commitment in that window
> When options are generated
> Then that transfer is not offered as a remedy
> And it is listed among rejected options with the reason

**AC 5.5: Applying a remedy is explicit**

> Given a proposed remedy
> When the planner accepts it
> Then it is applied as a lever on the plan and the order is recomputed
> And nothing is changed on the plan until that acceptance

### Story 6: Compare my order with the proposed one

**AC 6.1: The planner's own order is scored**

> Given a plan where every initiative carries a stated priority
> When the order is computed
> Then the engine also computes the schedule that follows the stated priority order exactly
> And reports both objective scores side by side

**AC 6.2: Every deviation is itemised**

> Given a proposed order that differs from the stated priority order
> When the planner opens the reconciliation view
> Then each moved initiative shows stated rank, proposed rank, the reason for the move, and the cost of keeping the stated rank instead
> And each row offers "pin this priority", which locks it and recomputes

**AC 6.3: Pinning everything degrades gracefully**

> Given a planner who pins every initiative's priority
> When the order is computed
> Then the schedule follows the stated order exactly
> And the report shows only date verdicts and constraint attributions, with no priority-change proposals

### Cross-cutting edge cases

**AC X.1: Cyclic dependencies**

> Given initiative work whose pod dependencies form a cycle
> When the order is computed
> Then the cycle is reported with the pods involved
> And the initiative is scheduled with the cycle broken at a named edge, marked as an assumption

**AC X.2: Pods referenced but not on the roster**

> Given an initiative depending on a pod that is not in the plan's roster
> When the order is computed
> Then that pod is treated as zero-capacity and the initiative is reported as unschedulable
> And the existing unknown-pods warning already shown by Plan is reused

**AC X.3: A freeze window covering a target date**

> Given a change-freeze window spanning the week of an initiative's target date
> When the order is computed
> Then the initiative's commit week is moved to the first week after the freeze
> And the verdict names the freeze as the cause

**AC X.4: Carryover work already in flight**

> Given an initiative marked in flight at 40% complete at period start
> When the order is computed
> Then only its remaining 60% consumes capacity
> And it is not held back by the org WIP limit, though it counts toward it

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The system MUST accept, per initiative: stated priority, priority-locked flag, target date, date-locked flag, requester tier, cost of delay per week, earliest start, predecessor initiatives, full-kit readiness percentage, in-flight flag and progress percentage | MUST |
| FR-002 | The system MUST accept these attributes both as optional columns in the uploaded initiatives matrix and through in-app editing, and MUST treat a plan lacking every one of them exactly as it behaves today | MUST |
| FR-003 | The system MUST produce, for each initiative, a proposed rank, start week, raw finish week, buffered commit week, and the constraint that determined its start | MUST |
| FR-004 | The system MUST produce, for each pod, utilization for every week of the period and the ordered list of work slices assigned to it | MUST |
| FR-005 | The system MUST NOT schedule more concurrent work slices at a pod than that pod's effective tracks in any week | MUST |
| FR-006 | The system MUST NOT start a work slice before its in-initiative predecessors have finished plus the cross-site handoff latency implied by the two pods' sites | MUST |
| FR-007 | The system MUST NOT start an initiative before its predecessor initiatives finish, before its earliest-start week, or below the configured full-kit readiness gate | MUST |
| FR-008 | The system MUST enforce a configurable limit on concurrent in-flight initiatives org-wide and per pod, and MUST report which initiatives each limit delayed | MUST |
| FR-009 | The system MUST treat each distinct named lead (PM, engineering, architect, programme) as a resource with a configurable concurrent-initiative capacity, and MUST report lead-bound initiatives distinctly from pod-bound ones | MUST |
| FR-010 | The system MUST treat stated priority as a strong preference that it may override, except where the initiative is priority-locked, in which case its proposed rank MUST equal its stated rank and it MUST win every resource contest against worse-priority initiatives | MUST |
| FR-011 | The system MUST rank against a weighted-lateness objective in which weight derives from cost of delay, requester tier and stated priority, and in which date-locked commitments dominate | MUST |
| FR-012 | The system MUST report, for every initiative whose proposed rank differs from its stated rank, the reason and the cost of retaining the stated rank | MUST |
| FR-013 | The system MUST also evaluate the schedule implied by the planner's stated priority order and report both objective scores | MUST |
| FR-014 | The system MUST classify each missed date as structurally infeasible (unachievable at unlimited capacity) or contention-driven | MUST |
| FR-015 | The system MUST generate ranked remedies for each missed date, at minimum: raise priority, descope, add capacity, transfer capacity, relax the date, defer another initiative — each with its recomputed verdict and its effect on every other initiative | MUST |
| FR-016 | Capacity transfer proposals MUST carry a plausibility rating derived from the two pods' working-hours overlap and their prior collaboration in the plan, MUST apply a ramp period at reduced effectiveness, and MUST remove the capacity from the donor immediately | MUST |
| FR-017 | The system MUST NOT propose a capacity transfer whose donor would exceed 0.85 utilization in the transfer window or that puts a date-locked commitment at risk, and MUST list such candidates as rejected with the reason | MUST |
| FR-018 | The system MUST support calendar constraints: per-site non-working windows, org-wide change-freeze windows that block initiative starts and completions, and capacity that becomes available part-way through the period | MUST |
| FR-019 | The system MUST size a buffer per initiative and report the buffered commit week separately from the raw finish week, and MUST express date verdicts against the buffered commit week | MUST |
| FR-020 | The system MUST NOT apply the existing queue multiplier m(ρ) inside a scheduled timeline, since finite-capacity scheduling makes waiting explicit | MUST |
| FR-021 | The system MUST report, for every initiative and pod, the inputs that produced its position: the ranking terms, the binding constraint, and any assumption applied (broken cycle, unestimated work, missing site data) | MUST |
| FR-022 | The system MUST leave the plan unchanged until the planner explicitly accepts a proposal or remedy | MUST |
| FR-023 | The system SHOULD present the order as both a ranked table and a time view showing each initiative's span, buffer and target date | SHOULD |
| FR-024 | The system SHOULD offer a fever-chart view of buffer consumption per initiative against its commit date, consistent with the existing Observe fever chart | SHOULD |
| FR-025 | The system SHOULD allow an accepted order to be saved as a named baseline and a later recomputation to be compared against it | SHOULD |
| FR-026 | The system SHOULD cap the number of initiatives that may start in any single quarter, to model the org's limited capacity to absorb change | SHOULD |
| FR-027 | The system MAY seed initiative attributes from Jira (epic due date, commitment label, requester tier) where the plan is linked to an imported snapshot | MAY |
| FR-028 | The system MUST NOT schedule below pod granularity or assign work to named individuals other than the lead-availability constraint in FR-009 | MUST NOT |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | Determinism | Identical inputs produce byte-identical output across runs and processes; no randomness, no map-iteration order dependence | Repeat-run equality test in CI |
| NFR-002 | Compute time | < 2s server-side for 200 initiatives × 60 pods × 104 weeks, including the multi-rule pass and the stated-order comparison | Benchmark test with a generated worst-case plan |
| NFR-003 | Remedy generation time | < 5s for the full remedy set across all missed dates in the same worst-case plan | Benchmark test |
| NFR-004 | Backward compatibility | A plan with none of the new attributes yields identical output from every existing Plan endpoint and view | Golden-file test against current demo plan output |
| NFR-005 | Aggregate consistency | Mean weekly utilization per pod equals the existing flat ρ within 2 percentage points where no idle is forced, and any larger gap is reported as attributed idle | Property test on the demo plan and generated plans |
| NFR-006 | Feasibility soundness | No produced schedule violates track capacity, dependency order, handoff latency, locks, WIP limits or calendar windows | Property test asserting all invariants on generated plans |
| NFR-007 | Explainability | Every initiative's start week traces to exactly one named binding constraint; every rank traces to its ranking terms | Assertion in schedule tests; visible in UI per FR-021 |
| NFR-008 | Estimate honesty | The UI states that weeks are estimates and that the order is a decision aid, not a forecast, consistent with existing Plan copy | Copy review |
| NFR-009 | Framing | No output attributes delay to a named individual; lead constraints are reported by role and initiative, never as personal throughput | Copy and output review |
| NFR-010 | Pure engine | Scheduling logic lives in a dependency-free package with no I/O, mirroring the existing `sim.js` and `planning` conventions | Package review; unit tests without a database |

---

## 7. Data Model

### Entities

**Initiative** _(extends the existing entity; all attributes optional)_
- statedPriority: integer — planner's rank, 1 = highest; 0 or absent = unranked
- priorityLocked: boolean — stated priority is non-negotiable
- targetDate: date — the date this initiative is wanted by
- dateLocked: boolean — the date is a commitment, not an aspiration
- tier: integer 1–4 — requester tier, per the existing tier semantics (T1 contractual … T4 aspirational)
- costOfDelayPerWeek: number — value lost per week late; unitless points are acceptable
- earliestStart: date — not before, for external funding, hiring or upstream events
- afterInitiatives: list of initiative names — cross-initiative precedence
- kitPct: number 0–1 — full-kit readiness at period start
- inFlight: boolean, progressPct: number 0–1 — carryover at period start

**SchedulingParams** _(plan-level)_
- periodStart: date — maps to week 0
- maxConcurrentInitiatives: integer — org WIP limit
- maxInitiativesPerPod: integer — per-pod concurrency cap
- kitGate: number 0–1 — minimum readiness to release
- targetUtilization: number — the ceiling the release rule staggers against
- bufferPct, feedingBufferPct: number — buffer sizing
- maxStartsPerQuarter: integer — change-absorption cap
- leadCapacity: map of role → integer — concurrent initiatives per named lead
- allowTransfers: boolean, transferRampWeeks: integer

**CalendarWindow**
- kind: enum (site-nonworking, change-freeze, event)
- scope: site name or org-wide
- fromDate, toDate: date
- effect: enum (reduce-capacity, block-start, block-finish)

**WorkSlice** _(derived; one per initiative × pod in path)_
- initiative, pod: references
- remainingWeeks: number — after carryover
- startWeek, finishWeek: integer
- waitWeeks: number — time between ready and started
- bindingConstraint: enum (dependency, handoff, pod-capacity, lead, wip-limit, kit-gate, freeze, earliest-start, predecessor)

**ScheduledInitiative** _(derived)_
- proposedRank, statedRank: integer
- startWeek, rawFinishWeek, commitWeek: integer
- bufferWeeks, bufferConsumedPct: number
- verdict: enum (on-time, at-risk, late, structurally-infeasible, unschedulable, provisional)
- weeksLate: number, bindingConstraint, rankingTerms, assumptions

**Schedule** _(derived; the whole result)_
- initiatives: list of ScheduledInitiative
- podWeeks: per pod, per week utilization and occupying slices
- drumPods: the constraint pods the release rule staggers against
- objectiveScore, statedOrderObjectiveScore: number
- reconciliation: list of rank deviations with reason and cost
- conflicts: list of conflicting locked pairs
- rejectedTransfers, assumptions, warnings

**Remedy** _(derived, proposed only)_
- kind: enum (raise-priority, descope, add-capacity, transfer-capacity, relax-date, defer-other, unlock)
- target: initiative or pod, magnitude, plausibility (transfers only)
- resultingVerdict, objectiveDelta, affectedInitiatives with week deltas

**Baseline**
- name, createdAt, the accepted Schedule and the parameters that produced it

### Relationships

- A Plan has one SchedulingParams, many CalendarWindows and many Initiatives.
- An Initiative has many WorkSlices, one per pod in its path; a WorkSlice belongs to one pod.
- An Initiative may have many predecessor Initiatives.
- A Schedule has one ScheduledInitiative per Initiative and many podWeek entries.
- A Plan may have many Baselines.

---

## 8. API Contract

Additions follow the existing Plan API shape: stateless computation endpoints
that never mutate the plan, plus explicit save endpoints.

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| POST | /api/plan/{id}/schedule | Compute the execution order. Accepts optional draft initiatives and levers so an unsaved upload can be sequenced, matching the existing simulate/preview behaviour | scheduling params, optional initiatives override, optional levers | Schedule |
| POST | /api/plan/{id}/schedule/remedies | Generate ranked remedies for missed dates | schedule request plus target initiative(s) | list of Remedy |
| PATCH | /api/plan/{id}/initiatives | Edit the sequencing attributes of one or more initiatives in place | initiative name plus attributes from §7 | updated initiatives |
| PATCH | /api/plan/{id}/scheduling | Save plan-level scheduling params and calendar windows | SchedulingParams, CalendarWindows | ok |
| POST | /api/plan/{id}/baseline | Save the current schedule as a named baseline | name, schedule request | baseline id |
| GET | /api/plan/{id}/baseline/{bid} | Retrieve a baseline for comparison | — | Baseline |

The uploaded initiatives matrix gains optional columns recognised by header
name — priority, priority fixed, target date, date fixed, tier, cost of delay,
earliest start, depends on initiative, kit %, in flight, % complete — placed to
the left of the full-kit total column. Unrecognised columns are ignored, as
today.

---

## 9. Out of Scope

- Scheduling below pod granularity: no per-person assignment, no task-level plans, no individual capacity or velocity. Named leads are modelled only as a concurrency limit on how many initiatives can be *started*.
- Becoming a project-management tool. No dependency editing UI beyond the sequencing attributes, no percent-complete tracking during the period, no export to MS Project.
- Being a system of record for commitments. Dates entered here are planning inputs; Jira and Aha remain the commitment vehicles, per the commitment-modelling recommendation in `requester-tiers.md`.
- Optimisation search (metaheuristics, MILP). See Decision 1.
- Monte Carlo over the schedule. Uncertainty is represented by explicit buffers in this iteration; probabilistic finish distributions are a candidate follow-up.
- Continuous re-planning against actuals. The baseline comparison in FR-025 is manual, not a live burn-up.
- Automatic derivation of tier or cost of delay from Jira. FR-027 is a MAY, deferred to a later phase.
- Changing anything in the Observe or Train lenses. The plan-to-game seeding path stays as it is.

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Where does the working-hours overlap between two sites come from for plan rosters? Observe has a site overlap matrix from the pod directory; plan Teams carry only a site string. Reuse the directory matrix, add a site table to the plan, or fall back to a default same-site/different-site pair of values? | Anoop | — | [NEEDS CLARIFICATION] |
| Q2 | Cost of delay units: currency per week, or an unitless 1–10 scale? Recommendation is unitless, since real CoD is rarely known and the objective only needs relative weights. | Anoop | — | [NEEDS CLARIFICATION] |
| Q3 | Default concurrent-initiative capacity per lead role. Proposed defaults: PM 2, engineering lead 2, architect 3, programme 4. Are these plausible for the reference plan? | Anoop | — | [NEEDS CLARIFICATION] |
| Q4 | Can a pod run one initiative's work slice across two tracks to halve its duration? The spec assumes no (a slice occupies one track). If some pods genuinely parallelise within an initiative, the model needs a per-slice track count. | Anoop | — | [NEEDS CLARIFICATION] |
| Q5 | Buffer sizing: a flat percentage of the chain, or the classic square-root-of-sum-of-squares over slice estimates? Flat is easier to explain; SSQ rewards initiatives with many small slices. | Anoop | — | [NEEDS CLARIFICATION] |
| Q6 | Is one target date per initiative sufficient, or do initiatives carry intermediate milestones (early access, GA) with their own dates? | Anoop | — | [NEEDS CLARIFICATION] |
| Q7 | Should the org WIP limit default to a number, or be derived (for example, tracks at the constraint pod)? A wrong default here is the single most visible knob in the feature. | Anoop | — | [NEEDS CLARIFICATION] |
| Q8 | Does the priority column allow ties, and is 1 the highest? Spec assumes yes and yes, with ties broken by the ranking rule. | Anoop | — | [NEEDS CLARIFICATION] |

---

## 11. Decision Record

### Decision 1: A finite-capacity weekly scheduler, not waves and not a solver

**Context:** Ordering initiatives is meaningless without a time axis. The
current model has none: it spreads all demand over one flat horizon, which is
why every initiative implicitly runs for the whole period.

**Decision:** Introduce weekly time buckets. Pods become finite servers whose
tracks are the servers; an initiative's work at a pod becomes a job occupying
one track for its duration. Build the schedule with a serial
schedule-generation scheme driven by dispatch rules — the standard, well-studied
approach to resource-constrained project scheduling.

**Alternatives considered:**
- Ranked order plus waves, keeping the flat ρ maths — rejected because it cannot test a target date, which is the centre of what was asked.
- Optimisation search (genetic, annealing, MILP) — rejected because it is non-deterministic without careful seeding, hard to defend to a leadership audience ("why is my initiative third?"), and over-precise given FullKit estimate quality. A greedy multi-rule pass gets most of the value at a fraction of the explanation cost.

**Consequences:** Plan gains a real time dimension, which is a substantial
addition to a package that is currently one flat computation. The output is a
good schedule, not a provably optimal one; the multi-rule pass in Decision 6
mitigates the greedy anomalies that come with it.

### Decision 2: Rank with Apparent Tardiness Cost; WSJF is its no-date special case

**Context:** The chosen objective is weighted lateness. The classic dispatch
rule for exactly that objective is Apparent Tardiness Cost: rank by delay weight
per unit of processing time, discounted exponentially by how much slack remains
to the due date.

**Decision:** Rank eligible work by that index, where processing time is the
initiative's consumption of the *constraint* pods rather than its total weeks
(Goldratt's value-per-constraint-hour), and weight combines cost of delay,
requester tier and stated priority.

**Alternatives considered:**
- Pure WSJF / cost-of-delay-divided-by-duration — rejected as insufficient on its own because it ignores due dates entirely. Note that when no initiative has a date, the index degenerates exactly to WSJF, so this decision subsumes the WSJF intake direction already recorded as a candidate feature.
- Minimum-slack / critical-ratio only — rejected because it ignores value and will happily starve a high-value undated initiative forever.
- A single opaque weighted sum — rejected because it cannot be explained line by line, and FR-021 requires that it can.

**Consequences:** Ranking is a formula with named terms that can be shown in the
UI. It introduces a look-ahead tuning parameter, which becomes another knob.

### Decision 3: Stated priority is a strong prior; locks are constraints

**Context:** The planner will have a priority column. Treating it as gospel
makes the tool a renderer of existing opinions; ignoring it makes the tool
arrogant and unusable.

**Decision:** Priority enters the ranking weight, so it shapes the order without
determining it. An initiative marked priority-locked has its proposed rank
pinned to its stated rank and wins every direct resource contest against
worse-priority initiatives. Every deviation from the stated order is reported
with its reason and the cost of not deviating.

**Alternatives considered:**
- Hard by default, unlockable — rejected because most plans would return "infeasible" with no proposal attached, which is the answer the planner already has.
- Tiebreak only — rejected because silently reordering the portfolio reads as ignoring the planner, and the trust cost is not worth the small objective gain.

**Consequences:** The reconciliation report becomes the primary artefact of the
feature, not a footnote. Locking is a first-class action in the UI, and a plan
where everything is locked must still produce a useful report (AC 6.3).

### Decision 4: Do not apply the queue multiplier inside a scheduled timeline

**Context:** The current model multiplies each pod's contribution by m(ρ) =
1/(1−ρ) to represent queueing. Under finite-capacity scheduling, waiting is
*produced* by the schedule: a slice waits because every track is busy.

**Decision:** Inside the schedule, use raw durations inflated only by the
existing capacity-loss factor, and represent residual variability as explicit
buffers. The existing flat-ρ view keeps m(ρ) unchanged.

**Alternatives considered:**
- Keep m(ρ) inside the timeline — rejected as double-counting: the delay would appear once as scheduled waiting and again as an inflated duration.
- Drop buffers and rely on the schedule alone — rejected because the schedule would then read as a promise, which contradicts the app's existing and correct framing that lead time is directional.

**Consequences:** Two lead-time numbers now exist in Plan, computed differently.
NFR-005 requires them to agree in aggregate, and the UI must explain why the
sequenced view can differ from the flat view for an individual initiative.

### Decision 5: Release by drum-buffer-rope, not by earliest possible start

**Context:** A scheduler that starts everything as early as capacity allows
reproduces the exact pathology the org is trying to escape — too much started,
nothing finishing.

**Decision:** Gate the *release* of an initiative on: the org WIP limit, the
per-pod concurrency cap, the full-kit readiness gate, freeze windows, the
change-absorption cap, and a stagger that holds planned load at the constraint
pods below a target utilization. Slices already released run as early as
capacity permits.

**Alternatives considered:**
- Earliest-start scheduling — rejected as above; it also produces schedules that look better on paper and worse in reality.
- WIP limit only — rejected because a readiness gate is the specific lever the org's own material argues for, and it is nearly free once initiatives carry a kit percentage.

**Consequences:** Some initiatives will show start weeks later than capacity
strictly requires, and the reason must always be named (FR-008), or the planner
will read it as a bug.

### Decision 6: Run several dispatch rules and keep the best, including the planner's own order

**Context:** Greedy scheduling has anomalies: adding capacity can occasionally
make a particular schedule worse, and no single dispatch rule wins on every
instance.

**Decision:** Run the schedule generator under several rules — the tardiness
index, minimum slack, value per constraint week, constraint-first, and the
planner's stated priority order — and keep the best by the objective, reporting
which rule won. The stated-order run is always computed, since it is what
Story 6 compares against.

**Alternatives considered:**
- One rule — rejected: cheaper but leaves obvious wins on the table and makes anomalies more visible.
- Full search — see Decision 1.

**Consequences:** Compute cost multiplies by the number of rules, which NFR-002
must absorb. In exchange, the tool can say "your order costs 14 weighted weeks
late, this one costs 3", which is the most persuasive output the feature has.

### Decision 7: Capacity transfer is a rated proposal, never a free move

**Context:** Moving capacity between pods is a genuine lever, and the obvious
naive version ("Delta is hot, Beacon has slack, move two tracks") is wrong often
enough to discredit the tool.

**Decision:** Model transfer with a plausibility rating from the two sites'
working-hours overlap and the pods' prior collaboration in this plan, an
asymmetric cost (the donor loses capacity immediately, the recipient gains it
after a ramp, at reduced effectiveness), and a refusal to propose transfers that
push the donor above the amber threshold or endanger a locked commitment.

**Alternatives considered:**
- Free instantaneous transfer — rejected as the "add people to a waiting problem" fallacy the org's own material calls out.
- Blocking transfers between low-overlap sites entirely — rejected because sometimes it is still the right move; a rating lets a human weigh it.

**Consequences:** Requires site overlap data for plan rosters (Q1) and a
familiarity signal, both of which need a source decision before this part can be
built. This is the last phase for that reason.

### Decision 8: Named leads are unit-capacity resources

**Context:** The FullKit sheet already names PM, engineering, architect and
programme leads per initiative, and this data sits unused. In practice one
architect fronting four simultaneous starts is a real and invisible constraint
on how much can begin at once.

**Decision:** Treat each distinct named lead as a resource with a small
concurrent-initiative capacity, constraining initiative starts. Report
lead-bound initiatives separately from pod-bound ones.

**Alternatives considered:**
- Ignore leads — rejected: the data is free and the constraint is real.
- Model leads with fractional allocation per initiative — rejected as more precision than the input supports.

**Consequences:** Sensitive framing. Output must name roles and initiatives, not
personal throughput (NFR-009). The demo plan currently populates no leads, so
sample data needs extending for this to be visible.

### Decision 9: Commit dates come from buffered finishes

**Context:** A raw scheduled finish week reads as a promise and will be quoted
as one.

**Decision:** Report a raw finish and a buffered commit week per initiative,
express every date verdict against the buffered week, and expose buffer
consumption in the same fever-chart form Observe already uses.

**Alternatives considered:**
- Raw finish only — rejected as a false promise.
- Cutting slice estimates in half and rebuilding them as buffer, per strict critical-chain practice — rejected because FullKit estimates are not documented as padded safe estimates, so halving them would be arbitrary.

**Consequences:** Adds a buffer-sizing parameter (Q5) and one more number per
initiative, offset by consistency with an idiom the app already teaches.

### Decision 10: Call the feature "Execution order", never "Sequence"

**Context:** The initiatives matrix already uses "<Team> Sequence" columns for
*dependencies*. Reusing the word for ordering would collide with an established
meaning in the same screen and the same file format.

**Decision:** Use "Execution order" in the UI, "schedule" in code, and keep
"sequence" reserved for the dependency columns.

**Consequences:** None, provided it is applied consistently from the start.

### Decision 11: New sheet columns are optional and additive

**Context:** The matrix parser locates team columns by finding the full-kit
total column and treating everything to its right as paired team columns.

**Decision:** New attribute columns are recognised by header name and must sit
to the left of the full-kit total. Absent columns mean absent attributes; a
sheet that predates this feature parses exactly as it does today.

**Alternatives considered:**
- A second sheet for sequencing attributes — rejected as one more thing to keep in sync by hand.
- In-app entry only — rejected in the requirements discussion: planners want the spreadsheet to stay the source of truth.

**Consequences:** Header detection must not mistake an attribute column for a
team column, which is a specific test case.

### Decision 12: Distinguish structural infeasibility from contention

**Context:** "This date will not be met" invites the wrong argument when the
real answer is "no ordering could ever meet it".

**Decision:** Test each dated initiative alone at unlimited capacity. If it
still misses, the date is structurally infeasible and only scope, an earlier
start or a later date can fix it. Otherwise the miss is contention, and
reordering, capacity or transfer are live options.

**Consequences:** One extra cheap pass, and a materially better conversation.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Dated initiatives whose feasibility is tested before commitment | 0% (no dates in the model) | 100% of dated initiatives in a plan | Count of dated initiatives with a verdict, per plan |
| Planning cycles ending with a defensible order | Order argued in a spreadsheet | Order produced and accepted in Conway, with deviations itemised | Baselines saved per planning period |
| Date misses discovered during planning rather than in-period | Discovered in-period | Majority discovered at plan time | Missed dates flagged at plan time vs raised as escalations later |
| Constraint pods sequenced by value rather than arrival | Not visible | Drum pods identified and their slice order explained in every plan | Presence of a drum set and per-slice constraint attribution in saved schedules |
| Priority disagreements resolved with a number | Opinion-based | Each deviation carries a cost figure | Reconciliation rows with a cost, per accepted schedule |

---

## Review Checklist

- [x] Problem is clearly stated and justified
- [x] User stories represent real user value
- [x] Acceptance criteria are in Given/When/Then format
- [x] Edge cases and error scenarios are covered (cycles, unknown pods, unestimated work, conflicting locks, past dates, freeze windows, carryover)
- [x] Requirements use MUST/SHOULD/MAY language
- [x] Non-functional requirements have measurable thresholds
- [x] Out of Scope is explicit
- [ ] Open questions are marked, owned, and time-bound — owners assigned, target dates pending
- [x] No implementation details in the requirements (algorithms confined to the Decision Record)
- [x] AI can read this spec (markdown, in the repo)
