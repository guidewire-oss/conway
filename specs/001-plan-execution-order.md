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

An accepted order is saved as an immutable **baseline** and opens as a
**timeline**: a portfolio view of every initiative's span, and a per-pod view
telling each team when its work starts, when it must start at the latest, and
how much slack it has. Once the period is running, the baseline is compared
against **actuals** mined from the Jira epics bound to each initiative, so the
question shifts from "what should the order be" to "where are we drifting from
it, and were our estimates honest". Wireframes for all of this are in §13.

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
retrospective language for this, from the flow review that motivated Conway, is
*"too many priorities means no priority"*, and *"these started, the clock ran,
the work never began"*.

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

### Story 7: Save the order as a baseline

**As a** planning manager
**I want** to save an accepted order, with the inputs and parameters that produced it, under a name
**So that** the period has one agreed reference that later re-plans and actuals are measured against

### Story 8: Open the plan as a timeline

**As a** planning manager or leadership audience
**I want** one button that opens the saved order as a timeline of every initiative and the pods inside it
**So that** I can see the shape of the period — overlaps, gaps, buffers and dates — in a single screen

### Story 9: See one team's work in order

**As a** pod lead
**I want** my pod's own timeline: every slice assigned to me, in order, with the latest date I can start each without hurting anyone downstream
**So that** I know what to start, when, and how much slack I actually have

### Story 10: Track actuals against the baseline

**As a** planning manager or PM
**I want** the baseline compared against what the Jira epics actually did
**So that** I can see where we are drifting, which estimates were wrong and by how much, and carry that correction into the next period

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

### Story 7: Save the order as a baseline

**AC 7.1: A baseline captures everything needed to reproduce it**

> Given a computed order the planner accepts
> When they save it under a name
> Then the baseline stores the schedule, the scheduling parameters, the initiative attributes and the roster as they were at that moment
> And recomputing from the stored inputs reproduces the stored schedule exactly

**AC 7.2: Baselines are immutable**

> Given a saved baseline
> When the plan's initiatives, roster or parameters are later edited
> Then the baseline is unchanged
> And it is marked as diverged from the plan's current inputs

**AC 7.3: One baseline is active**

> Given several saved baselines on a plan
> When the planner marks one active
> Then actuals and variance are reported against that one
> And the others remain readable as history

**AC 7.4: A new plan can be compared against a baseline**

> Given an active baseline and a freshly computed order
> When the planner compares them
> Then each initiative shows baseline start/commit versus current start/commit and the delta in weeks
> And initiatives added or removed since the baseline are listed separately

### Story 8: Open the plan as a timeline

**AC 8.1: The timeline opens from the order view in one action**

> Given a computed or saved order
> When the planner presses the timeline button
> Then the timeline opens showing one row per initiative across the period, with bar = scheduled span, appended buffer segment, and a target-date marker where one exists

**AC 8.2: The whole period always fits the width**

> Given any horizon from 4 to 104 weeks
> When the timeline renders at any viewport width down to 1024px
> Then the entire period is visible without horizontal scrolling
> And the time axis aggregates (weeks, fortnights, months) so that column labels never overlap

**AC 8.3: Rows fit the height or degrade predictably**

> Given a plan with more initiatives than fit at the default row height
> When the timeline renders
> Then rows are shown at a reduced density before any vertical scrolling is introduced
> And when even the minimum density does not fit, the row area scrolls with the time axis and row labels pinned

**AC 8.4: An initiative expands into its pods**

> Given an initiative row
> When the planner expands it
> Then one sub-row appears per pod slice in dependency order, each with its own span
> And the dependency arrows between those slices are drawn for that initiative only

**AC 8.5: Calendar and today context are visible**

> Given freeze windows, site non-working windows and a period in progress
> When the timeline renders
> Then freeze and non-working windows appear as marked vertical bands
> And today is marked with a line, positioned by date

**AC 8.6: Bars stay readable at any scale**

> Given a one-week slice on a 104-week horizon
> When the timeline renders
> Then that bar is still visible and clickable at a minimum width
> And labels that do not fit are truncated rather than overflowing their bar

### Story 9: See one team's work in order

**AC 9.1: A pod's slices are shown in scheduled order**

> Given a pod appearing in several initiatives
> When the planner or pod lead opens that pod's timeline
> Then every slice assigned to it is listed in start order, labelled with its initiative
> And concurrent slices are drawn in separate track lanes, never more lanes than the pod has tracks

**AC 9.2: Latest start and slack are explicit**

> Given a slice with slack between its earliest and latest possible start
> When the pod lead views it
> Then the row shows earliest start, latest start, and the slack in weeks
> And latest start is defined as the last week the slice can begin without moving its initiative's commit date

**AC 9.3: Zero-slack work is marked**

> Given a slice on its initiative's critical chain
> When the pod lead views it
> Then it is marked as having no slack
> And it is visually distinguished from slices that can safely wait

**AC 9.4: A pod lead sees what they are waiting on and who waits on them**

> Given a slice with upstream and downstream dependencies in other pods
> When the pod lead selects it
> Then the upstream pods and their finish weeks are named, including any cross-site handoff allowance
> And the downstream pods waiting on this slice are named

**AC 9.5: One team's view is shareable on its own**

> Given a pod lead who only cares about their pod
> When they open the pod view
> Then it renders without requiring the portfolio view to be open
> And it can be exported or linked so it can be taken to a team meeting

### Story 10: Track actuals against the baseline

**AC 10.1: Initiatives are bound to epics before any actuals are claimed**

> Given an initiative with no Jira epic bound to it
> When actuals are requested
> Then that initiative reports "not tracked" rather than a variance
> And it appears in a list of unbound initiatives with a prompt to bind

**AC 10.2: Binding is proposed but never silently applied**

> Given a Jira snapshot containing an epic whose summary closely matches an initiative's name
> When Conway proposes bindings
> Then the match is offered as a suggestion with its evidence and a confidence indicator
> And no binding takes effect until a human confirms it

**AC 10.3: Actuals are derived per pod slice, not only per initiative**

> Given an epic whose child issues carry pod assignments
> When actuals are computed
> Then each pod's actual start, actual finish and percent complete are derived from that pod's own child issues
> And they are compared against that pod's baseline slice

**AC 10.4: Schedule variance is reported in weeks against the baseline**

> Given a slice that the baseline scheduled to start in week 4 and whose first child issue actually started in week 7
> When variance is computed
> Then that slice reports a 3-week late start
> And the initiative aggregates its slices' variance into a schedule position

**AC 10.5: Estimate variance is separated from schedule variance**

> Given a slice estimated at 6 weeks that actually consumed 9 weeks of elapsed time
> When variance is computed
> Then the estimate variance is reported as +50% against estimate
> And it is reported separately from whether the slice started late

**AC 10.6: Estimate bias is aggregated per pod**

> Given several completed slices for one pod across initiatives
> When variance is computed
> Then that pod reports a systematic estimate bias
> And the bias is offered as a calibration factor for the next period's plan, never applied automatically

**AC 10.7: Order adherence is measured**

> Given a baseline order and the actual start dates of each slice
> When variance is computed
> Then the number of slices started out of baseline order is reported
> And the pods where it happened are named

**AC 10.8: Buffer consumption drives the status, not percent complete**

> Given an initiative that is 50% complete and has consumed 90% of its buffer
> When its status is shown
> Then it is reported in the red zone of a fever chart
> And the status is derived from buffer consumed against chain progress, not from percent complete alone

**AC 10.9: Scope change since the baseline is surfaced**

> Given epics added to or removed from an initiative's binding after the baseline was saved
> When variance is computed
> Then the added and removed work is listed as scope change
> And variance attributable to it is separated from variance against the original scope

**AC 10.10: Missing or poor Jira data degrades honestly**

> Given epics with no status-transition history, or children with no pod assignment
> When actuals are computed
> Then the affected figures are marked low-confidence with the specific gap named
> And no derived date is presented as measured when it was inferred

**AC 10.11: Refreshing actuals never rewrites the baseline**

> Given an active baseline and a fresh Jira pull
> When actuals are refreshed
> Then only the actuals and variance change
> And the baseline schedule, dates and estimates are untouched

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
| FR-025 | The system MUST allow an accepted order to be saved as a named baseline and a later recomputation to be compared against it | MUST |
| FR-026 | The system SHOULD cap the number of initiatives that may start in any single quarter, to model the org's limited capacity to absorb change | SHOULD |
| FR-027 | The system MAY seed initiative attributes from Jira (epic due date, commitment label, requester tier) where the plan is linked to an imported snapshot | MAY |
| FR-028 | The system MUST NOT schedule below pod granularity or assign work to named individuals other than the lead-availability constraint in FR-009 | MUST NOT |

### Baseline (Story 7)

| ID | Requirement | Priority |
|----|------------|----------|
| FR-029 | A baseline MUST store the schedule together with the scheduling parameters, initiative attributes, calendar and roster that produced it, such that recomputation from the stored inputs reproduces the stored schedule | MUST |
| FR-030 | Baselines MUST be immutable once saved, and MUST be flagged when the plan's current inputs have diverged from the stored ones | MUST |
| FR-031 | A plan MUST support several baselines with exactly one marked active; actuals and variance MUST be reported against the active baseline | MUST |
| FR-032 | The system MUST compare any computed order against a chosen baseline, reporting per-initiative start and commit deltas and listing initiatives added or removed since | MUST |
| FR-033 | The system SHOULD record who saved a baseline and when, so a period's agreed order is attributable | SHOULD |

### Timeline views (Stories 8 and 9)

| ID | Requirement | Priority |
|----|------------|----------|
| FR-034 | The system MUST provide a portfolio timeline: one row per initiative, bar spanning its scheduled weeks, an appended buffer segment, and a marker for its target date | MUST |
| FR-035 | The system MUST render the entire horizon within the container width at all supported viewport sizes, without horizontal scrolling; time zoom MUST be achieved by aggregating the axis (weeks, fortnights, months), never by widening beyond the container | MUST |
| FR-036 | The system MUST reduce row density before introducing vertical scrolling, and when scrolling is unavoidable MUST pin the time axis and the row labels | MUST |
| FR-037 | An initiative row MUST expand into one sub-row per pod slice in dependency order, with dependency arrows drawn for the expanded initiative only | MUST |
| FR-038 | The timeline MUST mark today, freeze windows, site non-working windows and period boundaries | MUST |
| FR-039 | Bars MUST have a minimum rendered width so short slices stay visible and clickable, and labels MUST truncate rather than overflow | MUST |
| FR-040 | The system MUST provide a per-pod timeline: one lane per track, slices placed in scheduled order and labelled by initiative | MUST |
| FR-041 | The per-pod view MUST show, for every slice, earliest start, latest start and slack in weeks, where latest start is the last week the slice can begin without moving its initiative's commit date | MUST |
| FR-042 | The per-pod view MUST mark zero-slack slices distinctly, and MUST name each slice's upstream dependencies (with any cross-site handoff allowance) and downstream waiters | MUST |
| FR-043 | The per-pod view MUST be reachable and readable on its own, without the portfolio view open, and SHOULD be exportable for use in a team meeting | MUST |
| FR-044 | Timeline colour MUST NOT be the only carrier of meaning; status MUST also be conveyed by position, pattern or label, and the palette MUST follow the app's existing red/amber/green utilization semantics | MUST |

### Actuals and variance (Story 10)

| ID | Requirement | Priority |
|----|------------|----------|
| FR-045 | The system MUST support binding each initiative to one or more Jira epics, resolved in this order of precedence: explicit epic keys entered by a human, a parent-epic hierarchy, a label convention, then a name-similarity suggestion | MUST |
| FR-046 | The system MUST NOT apply a name-similarity binding without human confirmation, and MUST show the evidence and a confidence indicator for every suggestion | MUST |
| FR-047 | The system MUST source actuals from an imported Jira snapshot rather than requiring a separate integration, and MUST record which snapshot and what time the actuals came from | MUST |
| FR-048 | The system MUST derive actual start, actual finish and percent complete per pod slice from that pod's own child issues, and per initiative by aggregation | MUST |
| FR-049 | The system MUST report schedule variance (baseline start/finish versus actual, in weeks) separately from estimate variance (estimated weeks versus consumed weeks, as a percentage) | MUST |
| FR-050 | The system MUST report buffer consumption against chain progress per initiative, and MUST derive the initiative's status from that relationship rather than from percent complete alone | MUST |
| FR-051 | The system MUST report order adherence: how many slices started out of baseline order, and in which pods | MUST |
| FR-052 | The system MUST aggregate estimate bias per pod across completed slices and offer it as a calibration factor for the next period, applied only on explicit acceptance | MUST |
| FR-053 | The system MUST identify scope change since the baseline (epics added to or removed from a binding) and separate variance attributable to it from variance against the original scope | MUST |
| FR-054 | The system MUST mark any actual that was inferred rather than measured as low-confidence, naming the specific data gap, and MUST NOT present an inferred date as measured | MUST |
| FR-055 | Refreshing actuals MUST NOT modify the baseline in any way | MUST |
| FR-056 | The system MUST report an initiative as "not tracked" where no binding exists, and MUST list unbound initiatives rather than omitting them | MUST |
| FR-057 | The system SHOULD account for capacity consumed by unplanned or interrupt work during the period, so estimate variance is not charged with time the pod never had | SHOULD |
| FR-058 | The system SHOULD detect roster drift during the period (pods gaining or losing capacity versus the baseline) and report it as a cause of variance | SHOULD |
| FR-059 | Variance output MUST be framed as system diagnosis: it MUST NOT attribute delay to named individuals, and MUST carry the same audit note the app's other leadership-visible metrics carry | MUST |

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
| NFR-011 | No horizontal scroll | The full horizon fits the container at viewport widths from 1024px to 2560px, for horizons of 4–104 weeks | Render test at both bounds for 4, 26, 52 and 104 weeks |
| NFR-012 | Vertical fit | Up to 30 collapsed initiative rows fit a 900px-tall viewport without scrolling, using density reduction; beyond that, scrolling with pinned axis and labels | Render test at 10, 30 and 80 initiatives |
| NFR-013 | Legibility floor | Minimum bar width 6px and minimum row height 8px; axis labels never overlap at any aggregation level | Visual regression test at each aggregation level |
| NFR-014 | Timeline render time | < 300ms to first paint for 200 initiatives expanded to 600 slices | Browser performance measurement |
| NFR-015 | Actuals freshness | Every actuals figure is labelled with its source snapshot and the time that snapshot was taken | Output review; assertion in actuals tests |
| NFR-016 | Colour independence | The timeline is fully interpretable in greyscale and passes contrast requirements in both light and dark themes | Greyscale render check; contrast audit |

---

## 7. Data Model

### Entities

**Initiative** _(extends the existing entity; all attributes optional)_
- statedPriority: integer — planner's rank, 1 = highest; 0 or absent = unranked
- priorityLocked: boolean — stated priority is non-negotiable
- targetDate: date — the single date this initiative is wanted by (Decision 21)
- dateLocked: boolean — the date is a commitment, not an aspiration
- tier: integer 1–4 — requester tier, per the existing tier semantics (T1 contractual … T4 aspirational)
- costOfDelayPerWeek: number — value lost per week late, expressed as unitless
  points on a 1–10 scale, because the ranking rule uses only the ratios between
  initiatives and never the absolute figure
- earliestStart: date — not before, for external funding, hiring or upstream events
- afterInitiatives: list of initiative names — cross-initiative precedence
- kitPct: number 0–1 — full-kit readiness at period start
- inFlight: boolean, progressPct: number 0–1 — carryover at period start

**SchedulingParams** _(plan-level)_
- periodStart: date — maps to week 0
- maxConcurrentInitiatives: integer — org WIP limit; absent or 0 derives it from
  the tracks at the drum pod (Decision 22)
- maxInitiativesPerPod: integer — per-pod concurrency cap
- kitGate: number 0–1 — minimum readiness to release
- targetUtilization: number — the ceiling the release rule staggers against
- wipModel: enum (strict, drum-gated, off) — which initiatives count against
  `maxConcurrentInitiatives`. Absent is *unchosen*, a reported state rather than a
  default: it schedules as `strict` so an existing plan's order does not move,
  while the response says the choice is outstanding and carries what each model
  would cost for this plan (Decision 22 as amended). `off` removes the org WIP
  limit only — the per-pod cap, the change-absorption cap, leads, tracks and
  dependencies all still apply
- bufferPct, feedingBufferPct: number — flat percentages of the chain and of each
  feeding path (Decision 20). Absent defaults to 0.25; an explicit value wins,
  including a 0, which commits on the raw finish
- maxStartsPerQuarter: integer — change-absorption cap
- leadCapacity: map of role → integer — concurrent initiatives per named lead.
  Optional: an omitted map, or a role absent from a supplied map, takes the
  defaults PM 2, engineering lead 2, architect 3, programme 4. A supplied role
  always wins, including a 0, which means that lead cannot take on an initiative
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
- bindingConstraint: enum (dependency, handoff, pod-capacity, lead, wip-limit,
  pod-wip-limit, starts-cap, kit-gate, freeze, earliest-start, predecessor).
  `wip-limit` is the org limit, `pod-wip-limit` the per-pod cap and
  `starts-cap` the change-absorption cap; FR-008 requires each to say which
  limit delayed the work, so they were split apart on 2026-08-20
- dependsOn: list of pod names — the in-plan, cycle-broken predecessors this
  slice waits on (FR-037's arrows, FR-042's upstream naming)
- latestStartWeek: integer — the last week the slice can begin without moving
  its initiative's commit date (FR-041)
- slackWeeks: integer — latestStartWeek − startWeek; zero marks the critical
  chain (AC 9.3, FR-042)

**ScheduledInitiative** _(derived)_
- proposedRank, statedRank: integer
- startWeek, rawFinishWeek, commitWeek: integer
- bufferWeeks, bufferConsumedPct: number
- verdict: enum (on-time, at-risk, late, no-date, structurally-infeasible,
  unschedulable). `no-date` is an initiative carrying no target date, which
  §13.2 already renders as "no date"
- provisional: boolean — a date verdict resting on unestimated in-path work
  (AC 2.5). Split out of the verdict enum on 2026-08-19: it qualifies any of
  the other values rather than replacing one, so an initiative can be both
  late and provisional
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
- name: string, createdAt: datetime, createdBy: string
- active: boolean — exactly one per plan
- schedule: the accepted Schedule, frozen
- inputs: the SchedulingParams, initiative attributes, calendar and roster as they were at save time
- inputsFingerprint: string — lets the plan's current inputs be flagged as diverged
- comparedTo: optional reference to the baseline this one superseded

**EpicBinding** _(initiative → Jira)_
- initiative: reference
- epicKeys: list of strings
- source: enum (manual, parent-epic, label, name-match)
- confidence: number 0–1 — only meaningful for name-match
- confirmedBy, confirmedAt — a name-match binding is inert until these are set
- addedAfterBaseline, removedAfterBaseline: lists of epic keys — the scope-change record

**SliceActual** _(derived, one per baseline slice)_
- initiative, pod: references
- actualStartWeek, actualFinishWeek: integer — measured where transition history exists, inferred otherwise
- percentComplete: number 0–1
- consumedWeeks: number — elapsed working weeks
- issueCounts: total, done, in progress, blocked
- confidence: enum (measured, inferred, unavailable) with a named reason when not measured

**SliceVariance** _(derived)_
- startVarianceWeeks, finishVarianceWeeks: number — actual minus baseline
- estimateVariancePct: number — consumed versus estimated
- startedOutOfOrder: boolean
- attributedTo: enum (contention, dependency, scope-change, interrupt, roster-drift, unknown)

**PodCalibration** _(derived, per pod per period)_
- completedSlices: integer
- estimateBias: number — systematic over- or under-estimate factor
- proposedCalibration: number — offered for the next period, never auto-applied

**PortfolioStatus** _(derived, per initiative)_
- chainProgressPct, bufferConsumedPct: number
- feverZone: enum (green, amber, red)
- schedulePositionWeeks: number — ahead or behind baseline
- scopeChange: summary of added and removed work

### Relationships

- A Plan has one SchedulingParams, many CalendarWindows and many Initiatives.
- An Initiative has many WorkSlices, one per pod in its path; a WorkSlice belongs to one pod.
- An Initiative may have many predecessor Initiatives.
- A Schedule has one ScheduledInitiative per Initiative and many podWeek entries.
- A Plan may have many Baselines; exactly one is active.
- An Initiative has zero or one EpicBinding; a binding names one or more Jira epics.
- A Baseline has one SliceActual and one SliceVariance per slice, and one PortfolioStatus per initiative, all derived from a named Jira snapshot.
- A Pod accumulates one PodCalibration per completed period.

---

## 8. API Contract

Additions follow the existing Plan API shape: stateless computation endpoints
that never mutate the plan, plus explicit save endpoints.

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| POST | /api/plan/{id}/schedule | Compute the execution order. Accepts optional draft initiatives and levers so an unsaved upload can be sequenced, matching the existing simulate/preview behaviour. Absent scheduling params fall back to the plan's saved policy; the response reports the WIP model in force and what each model would cost | scheduling params, optional initiatives override, optional levers | Schedule |
| POST | /api/plan/{id}/schedule/remedies | Generate ranked remedies for missed dates | schedule request plus target initiative(s) | `{remedies: list of Remedy, warnings: list of string}` — the envelope carries warnings, e.g. that transfer-capacity is deferred pending Q1 (§11 D7) |
| PATCH | /api/plan/{id}/initiatives | Edit the sequencing attributes of one or more initiatives in place | initiative name plus attributes from §7 | updated initiatives |
| PATCH | /api/plan/{id}/scheduling | Save plan-level scheduling params and calendar windows, including `wipModel` | SchedulingParams, CalendarWindows | ok |
| POST | /api/plan/{id}/baseline | Save the current schedule as a named baseline | name, schedule request | baseline id |
| GET | /api/plan/{id}/baseline | List baselines with their active flag and divergence state | — | list of Baseline metadata |
| GET | /api/plan/{id}/baseline/{bid} | Retrieve a baseline for comparison or timeline rendering | — | Baseline |
| PATCH | /api/plan/{id}/baseline/{bid} | Mark a baseline active, or rename it | active, name | ok |
| POST | /api/plan/{id}/baseline/{bid}/compare | Compare a computed order against this baseline | schedule request | per-initiative deltas, added and removed initiatives |
| GET | /api/plan/{id}/bindings | List initiative-to-epic bindings and unbound initiatives | — | list of EpicBinding |
| POST | /api/plan/{id}/bindings/suggest | Propose bindings from a snapshot, by hierarchy, label and name similarity | snapshot id | suggestions with evidence and confidence |
| PATCH | /api/plan/{id}/bindings | Confirm, edit or remove a binding | initiative, epic keys, source | updated EpicBinding |
| POST | /api/plan/{id}/baseline/{bid}/actuals | Recompute actuals and variance against a snapshot | snapshot id | SliceActual, SliceVariance, PortfolioStatus, PodCalibration |
| GET | /api/plan/{id}/baseline/{bid}/actuals | Retrieve the last computed actuals with their snapshot provenance | — | as above, plus source snapshot and timestamp |

The uploaded initiatives matrix gains optional columns recognised by header
name — priority, priority fixed, target date, date fixed, tier, cost of delay,
earliest start, depends on initiative, kit %, in flight, % complete — placed to
the left of the full-kit total column. Unrecognised columns are ignored, as
today.

---

## 9. Out of Scope

- Scheduling below pod granularity: no per-person assignment, no task-level plans, no individual capacity or velocity. Named leads are modelled only as a concurrency limit on how many initiatives can be *started*.
- Becoming a project-management tool. No dependency editing UI beyond the sequencing attributes, no percent-complete tracking during the period, no export to MS Project.
- Being a system of record for commitments. Dates entered here are planning inputs; the issue tracker and the roadmap tool remain the commitment vehicles.
- Optimisation search (metaheuristics, MILP). See Decision 1.
- Monte Carlo over the schedule. Uncertainty is represented by explicit buffers in this iteration; probabilistic finish distributions are a candidate follow-up.
- **Automatic** re-planning from actuals. Conway reports drift against the baseline and offers a per-pod calibration factor; deciding to re-plan, and accepting any calibration, stays a human action.
- A separate Jira integration for actuals. Actuals come from the snapshots the existing import already produces (§11 Decision 15), on demand — not from a live polling connection or a webhook.
- Writing anything back to Jira. Conway reads epics; it never updates dates, statuses or fields there.
- Task-level or sub-task-level tracking. Actuals resolve to the pod slice, from the epic's children; nothing finer.
- Automatic derivation of tier or cost of delay from Jira. FR-027 is a MAY, deferred to a later phase.
- Interactive Gantt editing. The timeline is a rendering of the computed order — bars are not dragged to reschedule. Changing the plan is done through attributes and levers, which recompute.
- Changing anything in the Observe or Train lenses. The plan-to-game seeding path stays as it is.

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Where does the working-hours overlap between two sites come from for plan rosters? Observe has a site overlap matrix from the pod directory; plan Teams carry only a site string. Reuse the directory matrix, add a site table to the plan, or fall back to a default same-site/different-site pair of values? | Anoop | 2026-08-18 | **Resolved 2026-08-18 — deferred.** Slices 1-2 apply no overlap factor. When it lands, the source is the plan-level site table in spec 003, not the pod directory. Blocked on spec 003 Q1-Q4. |
| Q2 | Cost of delay units: currency per week, or an unitless 1–10 scale? Recommendation is unitless, since real CoD is rarely known and the objective only needs relative weights. | Anoop | 2026-08-18 | **Resolved 2026-08-18.** Unitless points on a 1–10 scale. The objective needs relative weights only, and a currency figure nobody can source is worse than an honest score. |
| Q3 | Default concurrent-initiative capacity per lead role. Proposed defaults: PM 2, engineering lead 2, architect 3, programme 4. Are these plausible for a real org? | Anoop | 2026-08-18 | **Resolved 2026-08-18.** Defaults PM 2, engineering lead 2, architect 3, programme 4, each overridable per plan through `leadCapacity`. |
| Q4 | Can a pod run one initiative's work slice across two tracks to halve its duration? The spec assumes no (a slice occupies one track). If some pods genuinely parallelise within an initiative, the model needs a per-slice track count. | Anoop | 2026-08-18 | **Resolved 2026-08-18 — no.** A slice occupies one track. A per-slice track count is a later extension if a real pod is shown to parallelise within one initiative. |
| Q5 | Buffer sizing: a flat percentage of the chain, or the classic square-root-of-sum-of-squares over slice estimates? Flat is easier to explain; SSQ rewards initiatives with many small slices. | Anoop | 2026-08-18 | **Resolved 2026-08-18.** Flat `bufferPct` of the chain; SSQ stays available as a later parameter. See §11 D20. |
| Q6 | Is one target date per initiative sufficient, or do initiatives carry intermediate milestones (early access, GA) with their own dates? | Anoop | 2026-08-18 | **Resolved 2026-08-18.** One `targetDate` per initiative. See §11 D21. |
| Q7 | Should the org WIP limit default to a number, or be derived (for example, tracks at the constraint pod)? A wrong default here is the single most visible knob in the feature. | Anoop | 2026-08-18 | **Resolved 2026-08-18 — derived.** Absent or 0 means tracks at the drum pod. See §11 D22. |
| Q8 | Does the priority column allow ties, and is 1 the highest? Spec assumes yes and yes, with ties broken by the ranking rule. | Anoop | 2026-08-18 | **Resolved 2026-08-18 — yes and yes.** Ties are allowed, 1 is highest, and ties break on the ranking rule in §11 D2. |
| Q9 | Who owns the initiative-to-epic binding, and where is it entered — an Epics column in the FullKit sheet (stays with the planning artefact, rots between periods) or in-app on the plan (stays with the plan, invisible to the sheet's owner)? Recommendation is both, with the sheet winning on upload. | Anoop | 2026-08-18 | **Resolved 2026-08-18 — both entry points, and an uploaded sheet wins.** A binding may be entered in an Epics column of the FullKit sheet or in-app on the plan. When a sheet carrying that column is uploaded, its bindings replace the in-app ones for the initiatives it names; initiatives absent from the column keep their in-app bindings. This is about where a binding is *entered*, which the §11 D14 ladder does not cover — that ladder ranks how a binding is *resolved* once entered. See the D14 amendment. |
| Q10 | Is the Parent Epic convention actually in use in the target Jira yet — a dated parent epic carrying the commitment, with feature epics as its children? If so, binding is nearly free and target dates can be seeded from it. If not, every binding starts as manual entry. | Anoop | — | [NEEDS CLARIFICATION] — a fact about the target Jira, not a design choice, so it cannot be defaulted. the §11 D14 ladder works either way: without the convention every binding starts as manual entry and target dates are entered in-app. Needed before slice 6, not before slice 1. |
| Q11 | Does the Jira import capture changelogs? Without transition history, "actual start" can only be inferred (earliest child activity), which weakens every schedule-variance figure. `docs/v2-spec.md` lists changelog ingestion as a later phase — this feature is a strong reason to pull it forward. | Anoop | 2026-08-18 | **Resolved 2026-08-18 by inspection — not captured.** `server/jira/client.go:170` and `server/jira/detail.go:66` request `fields=` only, with no `expand=changelog`, so no transition history reaches a snapshot. Actual start is inferred and labelled as inferred. See §11 D23. |
| Q12 | Percent complete by issue count or by story points? Counts are always available; points are better but hygiene-dependent. Recommendation is counts, with points used where present and the choice labelled. | Anoop | 2026-08-18 | **Resolved 2026-08-18.** Issue counts, story points where present, and the basis labelled wherever a percentage is shown. |
| Q13 | How often should actuals refresh, and who triggers it — on demand from the plan, or automatically with each snapshot import? | Anoop | 2026-08-18 | **Resolved 2026-08-18 — on demand from the plan.** See §11 D24. |
| Q14 | An epic's children may carry pods that were never in the initiative's plan. Is that unplanned scope, a planning miss, or a mis-assignment? It changes whether it counts as variance or as scope change. | Anoop | 2026-08-18 | **Resolved 2026-08-18 — scope change, reported separately from variance.** See §11 D25. |
| Q15 | Can a pod lead be given access to only their own pod's timeline without seeing the whole portfolio? Today's roles are manager, admin and facilitator; a pod-lead role would be new. | Anoop | 2026-08-18 | **Resolved 2026-08-18 — no new role in this feature.** See §11 D26. |
| Q16 | Does any real FullKit workbook use the 1904 date system? `ReadXLSX` renders every cell as text and never reads its number format, so a date a planner formatted as a Date arrives as an Excel serial. `parseSheetDate` converts serials on the 1900 system, which is what Excel on Windows, Excel on modern macOS and Google Sheets all emit. A workbook saved with `date1904="1"` (legacy Mac Excel) would read four years and a day early, and nothing would flag it. Detecting it means parsing `xl/workbook.xml`'s `workbookPr`, which the grid reader does not surface — `ReadGrid` returns `[][]string` with no workbook metadata. | Anoop | — | [NEEDS CLARIFICATION] — recorded 2026-08-19 while implementing the §8 columns. Cheap mitigations if it matters: read the flag and pass it through, or reject a 1904 workbook at upload with a message telling the planner to re-save. Doing nothing is defensible if no such workbook exists in practice, and the failure is loud rather than subtle: every date in the plan shifts by four years, so verdicts would be absurd rather than plausibly wrong. Blocks nothing today. |
| Q17 | Is a WIP limit derived from drum tracks too tight in practice? Building the Order view made this visible on the demo plan: Delta has 2 tracks, so Decision 22 derives a limit of 2, and the portfolio then stretches to week 96 against a 26-week period, and of the seven dated initiatives five miss. The flat-rho view of the same plan reports Delta at rho 1.047 — demand roughly equal to capacity — so the two views disagree sharply, which is the tension NFR-005 requires not to exist in aggregate. The cause is that the org limit caps concurrent *initiatives* while the drum's tracks only constrain work *at the drum*: with a limit of 2, pods with slack sit idle waiting for a release slot. That is textbook drum-buffer-rope starving the non-constraints, so it may be correct and merely uncomfortable. | Anoop | 2026-08-20 | **Resolved 2026-08-20 — the planner picks the model.** Recorded from the demo plan's own output. All three readings are offered as `wipModel` — `strict`, `drum-gated`, `off` — with the measured cost of each shown for the planner's own plan, because the objective's preference for `off` is an artefact of Decision 4 rather than evidence. A plan that has not chosen is reported as unchosen and computed under `strict`, so nothing moves for an existing plan while the choice is demanded. See the §11 D22 amendment. |

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

### Decision 13: The baseline is immutable and self-contained

**Context:** Variance is only meaningful against a fixed reference. If the
baseline moves when the plan is edited, "we are three weeks late" quietly
becomes "we were always going to be here".

**Decision:** A baseline freezes the schedule *and* its inputs — parameters,
initiative attributes, calendar, roster. It is never edited. When the plan's
current inputs drift from the stored ones, the baseline is flagged as diverged
rather than updated. Re-planning creates a new baseline that records which one
it superseded.

**Alternatives considered:**
- Store only the schedule — rejected because a schedule whose inputs are gone cannot be re-derived or audited, and "why did it say week 4" becomes unanswerable.
- Auto-update the baseline on plan edits — rejected as the failure mode described above.

**Consequences:** Baselines are heavier to store (a full input snapshot each).
Cheap relative to the alternative, and it makes AC 7.1's reproducibility test
possible.

### Decision 14: Bind initiatives to epics through a precedence ladder, never a fuzzy guess

**Context:** An initiative is a spreadsheet row with a name; an epic is a Jira
key. Nothing connects them today, and actuals are impossible without that link.
Name matching is tempting and unreliable — "Telemetry GA" could match three
epics or none.

**Decision:** Resolve bindings by precedence: explicit epic keys entered by a
human first; then a parent-epic hierarchy where the org has adopted it; then a
label convention; then name similarity, which is only ever a *suggestion*
requiring confirmation. Unbound initiatives report "not tracked" rather than a
fabricated variance.

**Alternatives considered:**
- Name matching alone — rejected: a wrong binding produces confident, wrong variance, which is worse than no variance.
- Requiring the parent-epic hierarchy — rejected because Q10 is open; the ladder degrades to manual entry where the convention is not in use, rather than blocking.

**Consequences:** Someone has to do the binding work for the first period. The
ladder means that cost falls if the org adopts the parent-epic convention its
own commitment-modelling note already recommends.

**Amended 2026-08-18, where a binding is entered (§10 Q9):** the ladder above
ranks binding *sources* once a binding exists; it says nothing about where a human
enters one. Both locations are supported — an Epics column in the FullKit sheet,
and in-app on the plan — and an uploaded sheet wins for the initiatives its column
names, leaving in-app bindings intact for initiatives it does not mention. The
sheet wins because it is the artefact the planner edits deliberately, while an
in-app binding is often a one-off correction; a silent in-app override of a fresh
upload would be invisible to the person who owns the sheet.

### Decision 15: Actuals come from the existing snapshot pipeline, not a new integration

**Context:** Conway already imports Jira into Postgres as dated snapshots, with
per-issue pod assignment and status categories. Building a second, live Jira path
for actuals would duplicate authentication, rate limiting and field configuration.

**Decision:** Compute actuals from an imported snapshot, chosen explicitly, and
label every figure with which snapshot and when it was taken. Refreshing actuals
means importing a newer snapshot and recomputing.

**Alternatives considered:**
- A live Jira query per view — rejected: duplicated integration surface, rate-limit exposure, and figures that change under the reader mid-conversation.
- Asking PMs to enter actual dates by hand — rejected as the primary path: it is the data Jira already holds, and hand-entered actuals drift toward optimism. Retained only as the fallback for initiatives with no bindable epic.

**Consequences:** Actuals are as fresh as the last import, which must be stated
on screen (NFR-015). Snapshot cadence becomes a planning-hygiene question (Q13).

### Decision 16: Separate schedule variance from estimate variance

**Context:** "This slice took 9 weeks instead of 6" and "this slice started 3
weeks late" are different failures with different remedies, and collapsing them
into one number hides both.

**Decision:** Report them separately: schedule variance in weeks against the
baseline's start and finish, and estimate variance as a percentage of the
estimate. Attribute each where possible to contention, dependency, scope change,
interrupt load or roster drift. Aggregate estimate variance per pod into a bias
figure offered as next period's calibration.

**Consequences:** More numbers on screen, needing careful UI hierarchy (§13).
In exchange the planning loop closes: this period's measured bias improves next
period's estimates, which is the compounding value of the whole feature.

### Decision 17: Status comes from buffer consumption, not percent complete

**Context:** Percent complete is the most-quoted and least-informative progress
number: 90% complete with no buffer left is in trouble, 40% complete with most
of the buffer intact is fine.

**Decision:** Derive an initiative's status from buffer consumed against chain
progress, rendered as the fever chart Observe already uses, and show percent
complete as a supporting figure rather than the headline.

**Consequences:** Consistent with an idiom the app already teaches, and it makes
the buffers from Decision 9 do real work rather than being decoration.

### Decision 18: The timeline never scrolls horizontally; zoom is aggregation

**Context:** Every Gantt tool defaults to a fixed pixels-per-day scale and a
horizontally scrolling canvas. For a planning conversation this is the wrong
default: the whole point is seeing the period at once, and a reader who has to
scroll loses the overlaps that matter.

**Decision:** The horizon always fits the container width. Zooming out is done by
aggregating the time axis (weeks, fortnights, months) so labels stay legible;
zooming in filters the row set rather than widening the canvas. Vertically,
density reduces before scrolling starts, and if scrolling becomes unavoidable the
axis and row labels pin.

**Alternatives considered:**
- Conventional scrolling canvas with pixels-per-day zoom — rejected as above.
- Virtualised infinite canvas — rejected as far more machinery than a 26-to-104-week horizon needs.

**Consequences:** Bars for short slices get small, so a minimum width and
truncating labels are requirements (FR-039), not polish. A very long horizon with
very many initiatives will still need scrolling; the spec bounds when that starts
(NFR-012) instead of pretending it never happens.

### Decision 19: Two timeline lenses, one computed schedule

**Context:** A portfolio audience asks "what is the shape of the period"; a pod
lead asks "what do I start, and when at the latest". These want different rows
from the same data.

**Decision:** Ship both lenses over one schedule: by initiative (rows are
initiatives, expandable to pod slices) and by pod (rows are pods, lanes are
tracks, bars are slices coloured by initiative). The pod lens adds earliest
start, latest start and slack, which the portfolio lens does not need.

**Alternatives considered:**
- One view with a grouping toggle — rejected because the pod lens needs columns (latest start, slack, upstream/downstream) the portfolio lens has no room for.
- Pod view only — rejected: leadership needs the portfolio shape to make trade-offs.

**Consequences:** Two renderings to keep consistent. They must share one layout
computation, or they will drift and show different weeks for the same slice.

### Decision 20: Buffers are a flat percentage of the chain, not square-root-of-sum-of-squares

**Context:** CCPM sizes a project buffer from the variance of the chain's tasks,
classically the square root of the sum of squares of the individual safety
margins. Conway's slices are whole-week estimates entered in a spreadsheet, and
the fever chart only needs a denominator that everyone reads the same way.

**Decision:** `bufferPct` is a flat percentage of the chain length, and
`feedingBufferPct` the same for each feeding path. SSQ remains available as a
later parameter if a plan's estimates ever carry per-slice uncertainty.

**Alternatives considered:**
- SSQ from the start — rejected because it rewards an initiative for being
  chopped into many small slices, which is an artefact of how the sheet was
  filled in rather than a statement about risk, and no planner can reproduce the
  number in their head when they are asked why the buffer is nine weeks.
- No buffer, commit on the raw finish — rejected: the whole point of the buffer
  is that the raw finish is a fiction nobody should promise (Decision 4).

**Consequences:** Two initiatives with the same total length get the same buffer
regardless of shape, which is wrong in theory and legible in practice. If real
per-slice uncertainty ever arrives, the switch is a parameter, not a rewrite —
so keep buffer sizing behind one function.

**Amendment 2026-08-19 — the default is 25% of the chain.** D20 fixed the shape
of the buffer but recorded no value, and an absent `bufferPct` still has to
produce a commit week, because FR-019 and Decision 9 make the buffered week the
one every date verdict is read against. The default is 0.25.

Not 50%, the classic CCPM project buffer, because three layers of padding
already sit on the same uncertainty: FullKit weeks are the estimators' own safe
estimates, which is exactly why Decision 9 refused to halve them; `capacityLoss`
then inflates every slice duration for PTO and ramp; the buffer is the third.
Classic CCPM earns its 50% by stripping the first layer out first. Conway does
not, so 50% would pad the padding.

25% is also reproducible without a calculator — a 12-week chain buys a 3-week
buffer — which is the property D20 chose flat percentages for in the first
place. An explicit `bufferPct` always wins, including a 0, which means the
planner has chosen to commit on the raw finish.

---

### Decision 21: One target date per initiative

**Context:** Real initiatives often carry more than one committed date — early
access, then GA — and each could in principle have its own verdict.

**Decision:** One `targetDate` per initiative, with `dateLocked` marking a
commitment. Multiple milestones are out of scope for this feature.

**Alternatives considered:**
- A list of dated milestones per initiative — rejected for now: it multiplies
  every downstream surface (a verdict per milestone, a buffer per milestone, a
  fever chart per milestone) for a modelling gain nobody has asked for yet.
- Model the second date as a separate initiative with a precedence edge —
  available today via `afterInitiatives`, and honest: two commitments with
  different scope really are two pieces of work.

**Consequences:** An initiative with an early-access and a GA date must either
pick the one that governs sequencing or be split. The second option already
works, so this is a modelling instruction rather than a missing feature.

---

### Decision 22: The org WIP limit is derived from the drum, not defaulted to a number

**Context:** `maxConcurrentInitiatives` is the most visible knob in the feature.
A shipped default of, say, 5 is a number the tool cannot justify, and every
planner who sees it will either trust it or fight it.

**Decision:** When `maxConcurrentInitiatives` is absent or 0, derive it from the
tracks at the drum pod — the constraint the release rule already staggers
against (Decision 5). An explicit value always wins, and the UI shows which of
the two is in force.

**Alternatives considered:**
- A fixed default — rejected: it invents a capacity claim about an org the tool
  has never seen, and it is the one number a reviewer will quote back.
- No default, force the planner to choose — rejected: the first run of the
  feature would then need a value nobody has a basis for yet, which is a worse
  first impression than a derived one that can be explained in a sentence.

**Consequences:** The limit moves when the roster moves, which is correct and
also surprising, so the derived value must always be labelled as derived and
show the pod it came from.

**Amendment 2026-08-20 — the planner chooses the model, and is shown what each
one costs.** This decision fixed where the *number* comes from and left the
*rule* implicit: every initiative counts against the limit. Building the Order
view made the consequence visible, and §10 Q17 records it — on the demo plan a
2-track drum produces a limit of 2, the portfolio stretches to week 96 against a
26-week period, three pods sit idle for the whole period, and the flat-rho view
of the same plan reports demand roughly equal to capacity.

The cause is a mismatch of scope. The drum's tracks constrain work *at the drum*;
the org limit constrains *initiatives*, most of which never touch the drum. So
`wipModel` selects the rule, and all three are offered rather than one being
picked on the planner's behalf:

- `strict` — the limit is the drum's tracks and every initiative counts. Textbook
  drum-buffer-rope: protect the constraint absolutely, accept idle elsewhere.
- `drum-gated` — the limit is the drum's tracks, but an initiative counts only if
  it consumes a drum pod. Work that never touches the constraint flows freely.
- `off` — no org WIP limit. The per-pod cap, the change-absorption cap, leads,
  pod tracks, calendars and dependencies all still apply: this switches off one
  limit, not every limit. Also the mode under which AC 4.2 can be checked, since
  its precondition is "no idle enforced by WIP limits".

Measured on the demo plan, same roster and initiatives, only the model changed:

| model | portfolio ends | dates missed | pods idle all period | objective |
|-------|---------------|--------------|----------------------|-----------|
| strict | week 96 | 5 (2 late, 3 cannot fit) | 3 of 10 | 4673 |
| drum-gated | week 55 | 5 (2 late, 3 cannot fit) | 1 of 10 | 1466 |
| off | week 43 | 5 (2 late, 3 cannot fit) | 1 of 10 | 1155 |

Note what does *not* change: the same five dates are missed under every model, and
the same three of those can never fit. The models differ in what the misses
*cost* — a third of the weighted lateness between strict and drum-gated — and in
how much of the org is left idle to achieve it. Choosing a model buys cheaper
misses and busier pods, not fewer misses.

**Why this is the planner's choice and not the tool's.** The objective prefers
`off`, and that is not evidence that `off` is right — it is an artefact of
Decision 4, which removed the queue multiplier from the scheduler. Nothing here
charges for multitasking, context switching or queue variability, which is
precisely what a WIP limit exists to prevent. The limit therefore encodes a belief
about the organisation that the schedule cannot confirm or refute, so a tool that
silently picked one would be asserting something it cannot support.

**Unchosen is a state, not a default.** A plan that has not chosen still gets a
schedule, because AC 1.1 requires ranks, starts and finishes to appear on a plan
with no attributes at all. It is computed under `strict`, which changes nothing
for a plan that already had an order, and the response reports the model as
unchosen and carries the comparison, so the UI can demand the choice without
withholding the answer.

**Consequences of the amendment:** three schedules are computed per request
instead of one, so the comparison is of the planner's own plan rather than a
worked example. Two plans on different models are not comparable, so a baseline
must record the model it was computed under (Story 7). The three "cannot fit"
initiatives are identical under all three models, which is Decision 12 doing its
job: no release rule rescues a chain longer than the period.

---

### Decision 23: Actual start is inferred, and labelled as inferred

**Context:** Schedule variance wants the date work actually started. That comes
from a status transition, and the Jira client requests `fields=` only — see
`server/jira/client.go:170` and `server/jira/detail.go:66` — so no changelog
reaches a snapshot today.

**Decision:** Infer actual start from the earliest child activity in the bound
epic, and label every figure derived from it as inferred. Do not present an
inferred start as an observed one.

**Alternatives considered:**
- Pull changelog ingestion forward into this feature — rejected as scope: it is
  a snapshot-pipeline change with its own volume and rate-limit questions
  (`docs/v2-spec.md` has it as a later phase). This decision is a strong reason
  to prioritise it, and that argument is better made with the variance view
  already shipped and visibly hedged.
- Omit schedule variance until changelogs exist — rejected: an inferred start
  still catches the case the feature exists for, an initiative that began weeks
  after its baseline said it would.

**Consequences:** Schedule variance is directional, not exact, until changelog
ingestion lands. Every surface that shows it carries that qualification, and the
qualification is removed in one place when the data improves.

---

### Decision 24: Actuals refresh on demand, from the plan

**Context:** Actuals could recompute with every snapshot import, or when someone
asks for them.

**Decision:** On demand from the plan, through
`POST /api/plan/{id}/baseline/{bid}/actuals`, recording the source snapshot and
timestamp with the result.

**Alternatives considered:**
- Automatic on every import — rejected for this slice: an import serves Observe
  and has no idea which plans are bound to which epics, so it would do work
  nobody asked for and make the import slower for the common case.
- A schedule — rejected: nothing here is time-critical, and a cron makes
  provenance harder to read, not easier.

**Consequences:** A tracking view can be stale, so the stored snapshot id and
timestamp are part of the response and shown next to the figures rather than
implied.

---

### Decision 25: Pods outside the plan are scope change, not variance

**Context:** A bound epic's children may carry pods the initiative never planned
for. That is either a planning miss, a mis-assignment, or genuine new scope, and
the three want different treatment.

**Decision:** Report unplanned pods as scope change, in their own section,
separate from estimate and schedule variance. The estimate variance figures stay
about slices the plan actually contains.

**Alternatives considered:**
- Count them as estimate variance — rejected: it makes a pod look
  under-estimated when the truth is that nobody estimated it at all, and it
  quietly corrupts the pod calibration figures that variance feeds.
- Ignore them — rejected: unplanned pods are exactly the signal a planner wants
  before the next period.

**Consequences:** Three categories to keep distinct in the UI. Pod calibration
must exclude scope-change slices, or the calibration learns from work that was
never planned.

---

### Decision 26: No pod-lead role in this feature

**Context:** The per-pod timeline is written for a pod lead, and today's roles
are manager, admin and facilitator. A pod lead who should see only their own
lane would need a new role.

**Decision:** Ship the per-pod lens as a filtered view for the existing roles.
No new role, no new permission surface.

**Alternatives considered:**
- Add a pod-lead role now — rejected: a new role touches auth, JIT provisioning
  and the claim mapping, which is a larger and riskier change than the view it
  would gate, and it is not needed to prove the feature.

**Consequences:** A pod lead either gets manager access, which shows them the
whole portfolio, or receives their lane another way. If narrow access turns out
to matter, it is a separate spec against `server/auth`, not a change here.

---

---

### Decision 27: A baseline stores its inputs whole, and fingerprints all of them

**Context:** FR-029 requires a baseline to store enough that recomputing from it
reproduces the stored schedule, and FR-030 requires the plan's current inputs to be
flagged when they have diverged. Both need a decision the spec did not make: what
counts as "the inputs", and how divergence is detected.

**Decision:** Store the exact arguments `ComputeSchedule` was given as one frozen
blob alongside the resulting schedule, and fingerprint that blob whole with SHA-256
over its canonical JSON. No field list.

Mapped onto FR-029's terms so the requirement can be checked rather than
reconciled: the **roster** is the `[]Team`, the **initiative attributes** are the
`[]Initiative` (their §7 sequencing fields and their per-pod work), and the
**scheduling parameters** are `SchedulingParams` plus the two plan-level capacity
figures `Params` carries — `horizonWeeks` and `capacityLoss`, which live on the
plan rather than in `SchedulingParams` and are inputs to the schedule all the same.

FR-029's fourth term, the **calendar**, is not stored because nothing produces one
yet: `CalendarWindow` is specified in §7 but FR-018 is unimplemented, so there is
no value to freeze. When calendar windows land they join `BaselineInputs`, and
because the fingerprint covers the struct whole rather than a field list, they are
covered the moment they are added.

Reproducibility is then a property that can be tested rather than asserted:
recompute from the stored inputs and the result must match the stored schedule
byte for byte. A spec does exactly that.

**Alternatives considered:**
- Fingerprint only the fields that affect scheduling, so editing a description does
  not flag divergence — rejected. It requires a maintained list of
  "scheduling-relevant" fields, and the moment a new field is added and forgotten,
  a real change stops being detected. A false positive is a planner asking why it
  says diverged; a false negative is variance measured against inputs that silently
  moved. This session hit the field-list failure twice already, in `applyLevers`
  and in the sequencing-column writer.
- Store only the schedule and re-derive inputs from the plan — rejected by FR-029:
  a schedule whose inputs are gone cannot be reproduced or audited, which is the
  same reasoning Decision 13 gives.
- Store a database row per initiative rather than a blob — rejected: nothing
  queries inside a baseline. It is written whole, read whole, and compared whole,
  which is the same argument the `plans.scheduling` column already settled.

**Consequences:** Editing a plan's description marks its baselines diverged. That
is the conservative direction and it is visible, so a planner can ignore it; the
opposite error is invisible. Baselines are also larger than strictly necessary —
about 140KB for the ten-initiative demo plan — which is a fine price for being able
to answer "why did it say week 4" a quarter later.

**Two smaller choices recorded here rather than left to the reader:** the first
baseline a plan saves becomes active, because a plan with baselines and no active
one has no answer for "what are actuals measured against" (FR-031); and saving a
new baseline sets `comparedTo` to the previously active one, which is what §7's
"the baseline this one superseded" describes.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Dated initiatives whose feasibility is tested before commitment | 0% (no dates in the model) | 100% of dated initiatives in a plan | Count of dated initiatives with a verdict, per plan |
| Planning cycles ending with a defensible order | Order argued in a spreadsheet | Order produced and accepted in Conway, with deviations itemised | Baselines saved per planning period |
| Date misses discovered during planning rather than in-period | Discovered in-period | Majority discovered at plan time | Missed dates flagged at plan time vs raised as escalations later |
| Constraint pods sequenced by value rather than arrival | Not visible | Drum pods identified and their slice order explained in every plan | Presence of a drum set and per-slice constraint attribution in saved schedules |
| Priority disagreements resolved with a number | Opinion-based | Each deviation carries a cost figure | Reconciliation rows with a cost, per accepted schedule |
| Estimate quality improving period over period | Unknown — never measured | Per-pod estimate bias shrinking across periods | PodCalibration bias trend across baselines |
| Teams knowing their latest safe start | Not available | Every pod lead can see latest start and slack for their slices | Pod timeline views opened per period |

---

## 13. UI Representation

Wireframes, not visual design. They fix layout, information hierarchy and the
scaling behaviour required by FR-034 to FR-044; typography, spacing and exact
colour come later. All views reuse the app's existing red/amber/green
utilization semantics and its card/table idiom.

### 13.1 Where this lives

Plan gains a view switcher inside an open plan. Network is today's view; the
other three are new.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ← all plans   FY27 H1 — Platform          horizon [26]w  loss [10]%  [Save]  │
│                                                                              │
│  ⌗ Network    ≣ Order    ▦ Timeline    ◷ Actuals          baseline: ● v2 ▾   │
└──────────────────────────────────────────────────────────────────────────────┘
```

`baseline: ● v2 ▾` is always visible: which baseline is active, with a dot that
turns amber when the plan's inputs have diverged from it (FR-030).

### 13.2 Order view — the proposal and its reconciliation

The table that answers "what order, and where did you overrule me".

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Execution order          rule: tardiness-cost (best of 5)   [Open timeline ▸] │
│ your stated order: 14.0 weighted weeks late  ·  proposed: 3.5  ·  Δ −10.5     │
├──────────────────────────────────────────────────────────────────────────────┤
│ #  Initiative              Stated  Start  Commit  Target   Verdict    Binds   │
│ 1  Telemetry GA              2 →1    w1     w17    w20    ● on time   Delta   │
│ 2  Managed database MVP      4 →2    w1     w19    w18    ▲ late 1w   Delta   │
│    └ raise to #1 lands it, pushes Telemetry GA +3w        [options ▾]         │
│ 3  Self-service app platform 1 →3 🔒 w3     w22     —     ● no date   Delta   │
│ 4  Tenant isolation          5 →4    w6     w24    w22    ▲ late 2w   Ember   │
│ 5  Secrets rotation          3 →5    w9     w26     —     ● no date   Ember   │
│ …                                                                            │
├──────────────────────────────────────────────────────────────────────────────┤
│ ⚠ 1 structurally infeasible: DR for event streaming (chain 30w, target w12)   │
│                                          [Save as baseline]  [Open timeline ▸]│
└──────────────────────────────────────────────────────────────────────────────┘
```

`Stated 1 →3 🔒` reads as "you said 1, I propose 3" with a padlock to pin it
(AC 6.2). `Binds` names the constraint that set the start. The one-line
explanation appears under any row whose proposed rank differs from its stated
rank; `[options ▾]` expands the priced remedies (AC 5.1).

### 13.3 Timeline — portfolio lens (default, collapsed)

One row per initiative. The whole horizon fits the width; there is no
horizontal scrollbar at any horizon (Decision 18).

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ▦ Timeline    ◉ by initiative  ○ by pod      density [comfortable ▾]  [⤓ PNG]│
│                          Jan      Feb      Mar      Apr      May      Jun    │
│                       │w1  w5 │w6  w9 │w10 w13│w14 w17│w18 w21│w22 w26│      │
│                       ├───────┼───────┼───────┼───────┼───────┼───────┤      │
│ ▸ Telemetry GA        │███████████████████████░░░░│        ◆w20              │
│ ▸ Managed db MVP      │███████████████████████████░░░│      ◆w18             │
│ ▾ Self-service platf. │  ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░│                      │
│     └ Delta      10w  │  ████████████████│                                   │
│     └ Atlas       5w  │                  →████████│                          │
│     └ Cascade     4w  │                          →██████░░│                  │
│ ▸ Tenant isolation    │        ████████████████████████░░░░│    ◆w22         │
│ ▸ Secrets rotation    │              ██████████████████████░░│               │
│ ▸ BYO-auth (EA)       │                    ████████████████░░│               │
│                       │       ▒▒▒│               │           │  ░freeze░│    │
│                       └───────┴───────┴───────┴───────┴───────┴───────┘      │
│                              ↑today                                          │
│ █ scheduled   ░ buffer   ◆ target date   → handoff   ▒ site non-working       │
│ ░freeze░ change freeze   ▸ expand to pods                                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

Reading it: the bar is scheduled work, the lighter tail is the buffer, and the
diamond is the promise. A diamond sitting inside the buffer tail is a date with
no margin left; a diamond to the left of the tail's end is late — visible at a
glance without reading a number. `Self-service platform` is expanded, so its
pod slices show underneath with handoff arrows between them (FR-037).

### 13.4 Timeline — pod lens

Rows are pods, sub-lanes are tracks. A pod with 2 tracks can never show 3
stacked bars, which is the capacity constraint made visual.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ▦ Timeline    ○ by initiative  ◉ by pod       density [compact ▾]            │
│                          Jan      Feb      Mar      Apr      May      Jun    │
│                       ├───────┼───────┼───────┼───────┼───────┼───────┤      │
│ Delta      ρ1.05 ●    │                                                      │
│   track 1             │[Telemetry GA 12w ]│[Autoscaling 10w ]│               │
│   track 2             │[Self-serv 10w]│[Managed db 9w]│[DR 8w  ]│            │
│ Ember      ρ0.98 ▲    │                                                      │
│   track 1             │[Secrets 12w      ]│[Tenant isol. 13w    ]│           │
│   track 2             │[BYO-auth 10w  ]│[SCIM 11w        ]│                  │
│ Atlas      ρ0.31 ●    │                                                      │
│   track 1             │      [S-s 5w]│    [BYO 4w]│                          │
│   tracks 2–6          │  · idle ·                                            │
│ …                                                                            │
└──────────────────────────────────────────────────────────────────────────────┘
```

Idle tracks are shown, not hidden — visible slack on non-constraint pods is the
point the app already argues for, and hiding it would make every pod look busy.

### 13.5 Single pod — the team's own sheet

What a pod lead takes to their team. Reachable on its own (FR-043).

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ← timeline      Delta — 2 tracks · Remote · ρ 1.05 ● over capacity            │
│                 5 initiatives · 49w demand vs 46.8w capacity   [⤓ share]      │
├──────────────────────────────────────────────────────────────────────────────┤
│                       ├───────┼───────┼───────┼───────┼───────┼───────┤      │
│  track 1              │[Telemetry GA    ]│[Autoscaling     ]│                │
│  track 2              │[Self-service ]│[Managed db  ]│[DR event str.]│        │
├──────────────────────────────────────────────────────────────────────────────┤
│ Your work, in order                                                          │
│                                                                              │
│  Initiative          Weeks  Start   Start by   Slack  Waiting on  Blocks     │
│  Telemetry GA          12     w1       w1      none ⚠   —         Granite    │
│  Self-service platf.   10     w1       w1      none ⚠   —         Atlas      │
│  Managed database       9     w11      w13      2w      —         Harbor     │
│  Autoscaling rollout   10     w13      w13     none ⚠   —         Fjord      │
│  DR event streaming     8     w20      w18   late ▲     —         Ibis       │
│                                                                              │
│  ⚠ no slack: starting later moves the initiative's commit date               │
│  ▲ DR event streaming cannot start early enough — see options in Order       │
└──────────────────────────────────────────────────────────────────────────────┘
```

`Start by` is the answer to "when at the latest" (FR-041): the last week the
slice can begin without moving its initiative's commit date. `Blocks` tells the
team who is waiting on them, which is the thing pods most often cannot see.

### 13.6 Actuals — variance against the baseline

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ◷ Actuals    baseline ● v2 (saved 12 Jan)   snapshot: 14 Mar 09:12  [refresh]│
│ week 11 of 26 · 18 of 21 initiatives bound · 3 not tracked                    │
├──────────────────────────────────────────────────────────────────────────────┤
│  Initiative          Plan ▸ Actual        Sched   Est    Buffer  Status      │
│  Telemetry GA        ████████░░  planned  −0w    +8%     34%   ● on track    │
│                      ████████▓   actual                                      │
│  Managed db MVP      ███░░░░░░░  planned  +3w    +42%    91%   ▲ at risk     │
│                      ██▓░░░░░░   actual                                      │
│    └ Delta   9w est / 13w so far · started w7 vs w4 planned · contention     │
│  Self-service platf. ██████░░░░  planned  −1w    −5%     12%   ● on track    │
│  DR event streaming  ─ not tracked ─ no epic bound            [bind ▾]       │
├──────────────────────────────────────────────────────────────────────────────┤
│  Order adherence  4 of 17 slices started out of order (Delta ×2, Ember ×2)    │
│  Scope change     +2 epics on Tenant isolation since baseline (+6w est)       │
│  Estimate bias    Delta +38% · Ember +21% · Atlas −4%   [use for next plan]   │
│  ⓘ Actual start inferred for 3 slices — no transition history in Jira        │
│  ⓘ System diagnosis, not individual performance.                             │
└──────────────────────────────────────────────────────────────────────────────┘
```

Each initiative shows a doubled bar — planned above, actual below — so drift is
read by comparing two lines rather than parsing a number. `Sched` is weeks
against baseline, `Est` is percentage against estimate; they are deliberately
separate columns (Decision 16). Status comes from the buffer column, not from
completion (Decision 17).

### 13.7 Binding initiatives to epics

Reached from `[bind ▾]`, or as a bulk step after a snapshot import.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Bind initiatives to Jira epics          snapshot: 14 Mar 09:12               │
├──────────────────────────────────────────────────────────────────────────────┤
│  Telemetry GA          PLAT-4021    ✓ confirmed          via parent epic     │
│  Managed database MVP  PLAT-4088    ✓ confirmed          entered manually    │
│  Tenant isolation      PLAT-4130    ? suggested  78%     name similarity     │
│                        "Tenant isolation — phase 2" · Ember · 22 open        │
│                                                    [confirm]  [pick another] │
│  DR event streaming    — no candidate —                       [search Jira]  │
├──────────────────────────────────────────────────────────────────────────────┤
│  ⓘ Suggestions never bind on their own. 1 awaiting confirmation.             │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 13.8 Scaling rules the wireframes assume

| Situation | Behaviour |
|---|---|
| Horizon 4–104 weeks | Axis aggregates: ≤16w weekly labels, ≤40w fortnightly, beyond that monthly. Bars keep week precision regardless of label density |
| Viewport narrows | Time axis compresses first; row-label column has a floor, then truncates with a tooltip |
| More rows than fit | Density steps comfortable → compact → dense (row heights roughly 28 → 18 → 10px); only past dense does the row area scroll, with axis and labels pinned |
| Slice shorter than the minimum bar width | Rendered at the 6px floor with its label moved outside the bar |
| Initiative expanded | Only the expanded initiative's dependency arrows draw, so the view never becomes a web of lines |
| Many pods in the pod lens | Pods sort hottest-first by ρ, matching the Constraints table; below-threshold pods collapse into a "quiet pods" group |
| Export | The current lens renders to PNG or SVG at the on-screen layout, so what was discussed is what gets circulated |

---

## Review Checklist

- [x] Problem is clearly stated and justified
- [x] User stories represent real user value
- [x] Acceptance criteria are in Given/When/Then format
- [x] Edge cases and error scenarios are covered (cycles, unknown pods, unestimated work, conflicting locks, past dates, freeze windows, carryover, unbound initiatives, missing transition history, scope change)
- [x] Requirements use MUST/SHOULD/MAY language
- [x] Non-functional requirements have measurable thresholds
- [x] Out of Scope is explicit
- [ ] Open questions are marked, owned, and time-bound — owners assigned, target dates pending
- [x] No implementation details in the requirements (algorithms confined to the Decision Record)
- [x] AI can read this spec (markdown, in the repo)
