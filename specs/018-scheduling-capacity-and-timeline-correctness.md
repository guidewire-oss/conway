# Scheduling capacity and timeline correctness

**Status:** Implemented
**Author(s):** Project maintainers
**Date:** 2026-09-05
**Story/Ticket:** Local scheduling investigation
**Sprint/Cycle:** Current

## 1. Overview

Correct feasible effort allocation under a constraint-team utilization target,
and make every scheduled or rejected assignment visible in the timeline.
Preserve the uploaded inputs, baseline history and physical team capacities.

## 2. Problem

A sub-100% drum target currently rejects full-width effort slices after the
first placement even when the drum is idle. The first placement bypasses the
same check because its calendar does not exist yet. Separately, below-threshold
single-lane work can retain a duration calculated from all team lanes.
The timeline renders rejected initiatives as blank rows and can draw every
split lane using only the first phase, hiding real later work.

## 3. User Stories

### Story 1: Feasible effort schedules

As a planner I want capacity targets and chunk sizes to constrain actual
allocation, so valid work can be scheduled without inventing capacity.

### Story 2: Explain missing assignments

As a team lead I want scheduled phases and rejected assignments displayed
accurately, so an empty chart does not imply there is no planned work.

## 4. Acceptance Criteria

### Story 1

Given successive effort initiatives on a four-lane drum with a 90% target,
when capacity is available, then both may start within the period using at most
three drum lanes and duration covers the remaining effort after capacity loss.
The first initiative obeys the same budget as subsequent initiatives.
Given one physical lane and a 90% target, when work is scheduled, then the
unrepresentable sub-lane target is explicitly qualified and does not prohibit
all work. Target 0 or 1 preserves the existing no-stagger behavior.
Given 18 effort weeks below a 20-week chunk threshold on five physical lanes
with 10% loss, when placed on one lane, then duration is at least 20 weeks.
Given calendars, carryover, dependencies or unrelated teams, when applying the
target, then physical capacity and those constraints retain their meaning.
Given work placed ahead of a later initiative's dependency-ready time, then
new slices must not overlap it beyond the team's physical lane count.
Small effort estimates must cover capacity loss even when shorter than the
number of available lanes.

### Story 2

Given an initiative rejected at the horizon with no slices, when filtering
for a team assigned in its inputs, then the initiative remains discoverable
with its explicit rejection reason and no invented week-zero bar.
Given split phases of two lanes in weeks 2–6 and five lanes in weeks 6–10,
when rendered, then each phase occupies the correct weeks on its active lanes.
Given an empty two-lane team, then exactly two idle lane rows are shown.

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | Allocation MUST respect enforceable whole-lane drum budgets from the first release. | MUST |
| FR-002 | Work MUST NOT be rejected solely because its unconstrained lane choice exceeds a budget when a narrower feasible allocation exists. | MUST |
| FR-003 | Duration MUST conserve effort under selected lanes, capacity loss and applicable split overhead. | MUST |
| FR-004 | Sub-lane utilization targets MUST expose their representational limitation. | MUST |
| FR-005 | Physical calendars, carryover, precedence and non-drum behavior MUST remain valid. | MUST |
| FR-006 | Unscheduled assignments MUST remain visible under relevant team filters with a reason and unknown dates. | MUST |
| FR-007 | Timeline geometry MUST represent every lane phase at its actual time interval. | MUST |
| FR-008 | Empty teams MUST display exactly their physical lane count. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Regression safety | Existing Go/JS checks pass | Repository suites |
| NFR-002 | Privacy | No source spreadsheets or identifying fixtures committed | Staged diff inspection |
| NFR-003 | Input isolation | Investigation does not save the upload or alter agreement | Read-only reproduction |

## 7. Data Model

Retain current inputs and schedule shapes. Physical team capacity remains the
source for utilization reporting. Lane phases and binding reasons retain their
existing meanings. Warnings/assumptions may qualify an unrepresentable target.

## 8. API Contract

Existing schedule and preview endpoints are unchanged. Their derived schedules
now allocate work consistently and contain explanation text where necessary.

## 9. Out of Scope

Changing user inputs, saving an upload preview, removing valid lead/WIP limits,
a new optimization algorithm, fractional-week lanes, or pushing changes.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | How does a fractional target map to indivisible lanes? | Maintainers | 2026-09-05 | Decision 1; preserve strict enforceable occupancy and explicitly qualify the sub-lane exception. |

## 11. Decision Record

### Decision 1: Allocate against the enforceable drum budget

Spec 004 FR-007 promises an occupancy ceiling, not a long-term start-rate
average. Compute the admissible whole-lane budget as floor(target × physical
tracks) when 0 < target < 1 and the result is at least one. Effort placement
must choose lanes and recompute duration using that budget, rather than repeatedly
retrying an impossible full-width placement. Physical calendars continue to
reduce physical capacity; the usable capacity is the smaller bound, never a
percentage reduction applied twice. Calendar/heatmap reporting retains physical
tracks. A budget below one lane cannot be enforced by this model; ignore it for
that team with a visible assumption, matching the previous intended fallback.
Carryover already running is not retroactively un-started or silently narrowed.

Rejected: disable all staggering for effort work, let the first placement ignore
the target, or average load over an arbitrary window. Those contradict the
existing occupancy contract. Integer rounding can reserve more than the requested
fraction on small teams; make that limitation explicit.

### Decision 2: Conserve effort when lane choice changes

Use consistent lane selection for duration and placement, including chunk
thresholds and utilization budgets. A single-lane threshold cannot reuse the
shorter duration computed with all physical lanes. Tests assert consumed capacity,
not merely the lane-count attribute.
When effort allocation selects only one lane, waiting for that lane does not
divide the work. Use contiguous placement without a splitting overhead.
For an actual growth event, its first ramp week counts toward the configured
tax; it must not add an extra unconfigured week.
If growth happens during an earlier ramp, add the new event's tax to the
outstanding ramp rather than silently dropping either event's overhead.

### Decision 3: Distinguish assigned work from placed work

Use input-team membership to retain rejected initiatives in filtered views;
show a labeled unscheduled state rather than a zero-date bar. Render phase
intervals explicitly on their occupied lanes and keep physical idle lanes exact.
The chart remains a view of the server schedule, not a second scheduler.

### Decision 4: Respect future reservations while growing a slice

A growth-only split cannot keep its lanes through a future reservation that
leaves fewer lanes free. Reject that split attempt and use legal contiguous
placement instead of overriding already committed capacity. Non-working gaps
must close a phase so its displayed interval does not claim occupied capacity
during the gap.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Feasible full-width work refused by target | Reproduced | Zero in regression cases | Behavioral tests |
| Effort undercount below chunk threshold | Reproduced | Zero | Lane × duration assertion |
| Hidden unscheduled/phase intervals | Reproduced | Zero in renderer cases | JS and browser checks |

## Review Checklist

- Requirements recorded before correction.
- Generic regression fixtures only.
- Results must distinguish corrected defects from legitimate remaining limits.
