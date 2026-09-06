# Undated work and advisory lead capacity

**Status:** Done
**Author(s):** Project maintainers
**Date:** 2026-09-05
**Story/Ticket:** Schedule undated work on available team tracks
**Sprint/Cycle:** Current

## 1. Overview

Schedule estimated work without requiring supplied start or finish dates. Use
available team tracks and show calculated dates, while making assumed lead
workload limits advisory unless the planner explicitly enforces them.

## 2. Problem

Three undated initiatives on independent teams can leave the third team's track
empty because their shared product lead exceeds an implicit limit of two. The
message that dates are unknown can sound like missing dates caused the hold.
Ownership labels and default workload assumptions should not silently prevent
capacity-based planning.

## 3. User Stories

### Story 1: Schedule work before committing dates

As a planner I want estimated work placed in available stretches on team tracks
so I can discover feasible dates before making commitments.

### Story 2: Choose whether lead workload blocks work

As a planner I want explicit control over lead-limit enforcement so advisory
ownership assumptions do not become unexplained scheduling barriers.

## 4. Acceptance Criteria

### Story 1

**AC 1.1:** Given estimated initiatives without dates on independent teams,
when their shared lead exceeds the default workload threshold, then each can
start on a free track and the overload remains visible.

**AC 1.2:** Given a track with a later reservation, when an undated assignment
fits in the earlier contiguous free stretch, then it is placed in that stretch
without moving the reservation or overlapping other work.

**AC 1.3:** Given dependencies, calendar restrictions or explicit start pins,
when undated work is placed, then those constraints and physical capacity remain
enforced; a missing target date does not count as a missed deadline.

### Story 2

**AC 2.1:** Given an explicit hard lead limit, including zero, when it binds,
then work remains held with an explanation that missing dates are not the cause.

**AC 2.2:** Given advisory mode with an exceeded threshold, when the schedule
is returned, then affected overlapping initiatives carry a workload warning;
nonoverlapping work does not acquire an overload warning.

**AC 2.3:** Given saved policy, when a planner switches lead enforcement or
edits unrelated assumptions, then the selected mode and role thresholds survive.

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | Work MUST NOT require supplied start or finish dates to receive a placement. | MUST |
| FR-002 | Placement MUST respect physical capacity, dependencies, calendars and explicit scheduling constraints. | MUST |
| FR-003 | Implicit lead workload thresholds MUST NOT block work by default. | MUST |
| FR-004 | Planners MUST be able to explicitly enforce or advise on lead limits. | MUST |
| FR-005 | Advisory overload and hard-limit holds MUST be distinguishable. | MUST |
| FR-006 | Explicit limits in existing plans MUST retain their enforcement until changed. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Generic fixtures and documentation | No source identifiers | Diff review |
| NFR-002 | Scheduling integrity | No capacity violations in regressions or reference replay | Go tests and independent accounting |
| NFR-003 | Regression safety | Product suites pass | Go race tests and JavaScript tests |

## 7. Data Model

Scheduling policy gains optional `leadCapacityMode`: `advisory` or `hard`.
Existing `leadCapacity` values remain workload thresholds. Derived assumptions
carry advisory overload notices; no fabricated input dates are persisted.

## 8. API Contract

Existing scheduling read/write and preview endpoints carry the optional mode.
No endpoint or date-field requirement is added.

## 9. Out of Scope

Removing explicit WIP or utilization policy, ignoring dependencies, changing
estimates, modifying saved plans during the audit, and replacing the optimizer.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should lead workload block otherwise feasible team work? | Maintainers | 2026-09-05 | Default to advisory for implicit thresholds; retain explicit enforcement as a planner choice. |

## 11. Decision Record

### Decision 1: Separate assumed workload from an explicit release gate

Missing dates already schedule relative to the period origin; they are not the
cause of the reported hold. When mode is absent, plans without any explicit
role limits use advisory thresholds. Plans with an existing nonempty limit map
retain hard enforcement. An explicit mode overrides that inference. This
supersedes implicit hard enforcement in spec 019 Decision 5 while preserving
deliberately configured constraints, including zero.

### Decision 2: Keep warnings truthful across the completed schedule

In advisory mode, evaluate final overlapping lead occupancy and attach warnings
to every affected initiative, including earlier releases. Do not reuse failed
candidate explanations or claim a release was held. Hard mode keeps the current
bounded release search. Role normalization and placeholder handling are unchanged.

### Decision 3: Reuse legal contiguous placement

Undated work uses the existing earliest legal contiguous placement and economic
split comparison. A target date affects ranking and lateness, not eligibility.
No date-based exemption from real constraints is introduced: dated and undated
work follow the same lead policy. An unavailable placement explicitly describes
the binding constraint and says no dates were calculated, rather than suggesting
that dates must be supplied.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Independent teams blocked by implicit lead thresholds | One of three in reproduction | Zero in advisory mode | AC 1.1 |
| Capacity overlaps or lost hard constraints | Zero expected | Zero | Regressions and reference replay |

## Review Checklist

- Preserve explicit planner constraints.
- Exercise undated scheduling independently of lead-policy changes.
- Keep warnings and calculated dates distinct from input commitments.
