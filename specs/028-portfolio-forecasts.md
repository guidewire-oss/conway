# Portfolio forecasts

**Status:** In Progress
**Author(s):** Conway contributors
**Date:** 2026-09-07
**Story/Ticket:** Roadmap priority 2
**Sprint/Cycle:** Estimate ranges and evidence assessment

## 1. Overview

Managers can compare a saved plan under shorter estimates, its current estimates,
and longer estimates with shared disruption. All scenarios respect the planning
engine's constraints. Execution evidence helps assess how much historical support
exists without presenting modeled scenarios as calibrated probabilities.

## 2. Problem

A single finish date hides estimate uncertainty and shared capacity risk. A
manager needs to see which commitments are sensitive and which work has no
credible placement before negotiating scope or promising a date.

## 3. User Stories

### Story 1: Assess uncertainty before making a commitment

**As a** manager **I want** a whole-plan scenario comparison **so that** I can
identify fragile commitments while retaining dependencies and capacity limits.

### Story 2: Judge the evidence behind the forecast

**As a** manager **I want** explicit historical sample counts and gaps **so that**
I do not confuse a simulation with demonstrated prediction accuracy.

## 4. Acceptance Criteria

**AC 1.1:** Given saved planning inputs, when a manager runs a range, then three
named scenarios show all initiatives, commitments, incomplete placements, and
the assumptions used, without modifying inputs or agreements.

**AC 1.2:** Given missing estimates or work beyond the horizon, when scenarios
are compared, then unknown results remain unknown and whole-portfolio finish is
withheld when any initiative is provisional or lacks a complete placement.

**AC 1.3:** Given identical inputs and range settings, when the comparison is
repeated, then it reproduces the same schedules and input fingerprint.

**AC 1.4:** Given a manager changes plans, signs out, edits settings, or leaves
the forecast while a request is pending, then the late result cannot replace
the newer context or misrepresent which settings produced it.

**AC 2.1:** Given an accessible imported Jira snapshot and an active agreement, when
evidence is inspected, then inferred elapsed-time ratios show sample counts,
source and limitations separately from scenario dates. Missing or changed scope
must not supply calibration samples. These ratios must not be called effort
measurements, interval coverage, or proof of predictive calibration.

**AC 2.2:** Given unavailable or inaccessible evidence, when a manager requests
it, then an actionable error appears without silently retaining older evidence.

## 5. Functional Requirements

| ID | Requirement | Priority |
|---|---|---|
| FR-001 | Forecasts MUST belong to a named saved plan and expose the exact source fingerprint. | MUST |
| FR-002 | All scenarios MUST preserve dependency, track, readiness, WIP, calendar, ordering and buffer semantics. | MUST |
| FR-003 | Managers MUST control estimate factors and additional shared capacity loss within bounded valid ranges. | MUST |
| FR-004 | Results MUST distinguish finish before buffer from commitment including buffer and expose unplaced work. | MUST |
| FR-005 | Forecasts MUST NOT change a plan, baseline, imported evidence or execution decisions. | MUST |
| FR-006 | Historical evidence MUST name its source, age, sample size and inference limits. | MUST |
| FR-007 | The UI MUST provide contextual help, timeline access, error recovery and responsive accessible comparison. | MUST |
| FR-008 | Scenario dates MUST NOT carry probability or calibrated-confidence labels without historical prediction validation. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|---|---|---|---|
| NFR-001 | Determinism and isolation | No input mutation; repeatable output | Ginkgo/Gomega |
| NFR-002 | Accessible workflow | Keyboard forms; fits 360px width | Playwright |
| NFR-003 | Access control | Owner/admin only; no game or expired role access | Go integration |
| NFR-004 | Bounded computation | Three existing scheduler runs per request | Unit/integration |

## 7. Data Model

Forecast settings: lower estimate factor, upper estimate factor and additional
shared capacity loss. The result has four top-level fields: `fingerprint`,
`settings`, `scenarios` and `limitations`. Each scenario contains its name,
factor, disruption, nullable finish/commitment weeks, unknown count and full
`schedule`. Period start, horizon, initiative outcomes, warnings and assumptions
live inside that schedule. No new persistent source of planning truth is introduced.

## 8. API Contract

| Method | Path | Description | Request | Response |
|---|---|---|---|---|
| POST | /api/plan/{id}/forecast | Read-only scenario computation | lowerFactor, upperFactor, disruption | fingerprint, settings, scenarios, limitations |

The existing authorized actuals API supplies optional evidence. Malformed or
out-of-range settings return 400; inaccessible plans return 403/404; stale or
game-only roles return 403. Only one strict bounded JSON object is accepted.
Unreadable persisted inputs return a generic 500 response; diagnostic details
remain in server logs. Known input-validation failures retain actionable 400
responses through the shared scheduling error classification.

## 9. Out of Scope

- Predictive calibration claims, sampled probability distributions and P85 dates.
- Training on inferred issue starts as though they were measured effort.
- Applying scenario inputs to a plan automatically.

The remaining calibration increment needs immutable predictions issued before
observed outcomes, comparable cohorts and historical interval coverage. This
increment establishes the finite-resource range workflow; it does not complete
that validation requirement or change the roadmap order.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|---|---|---|---|
| Q1 | Which evidence can support predictive calibration today? | Maintainer | This increment | Existing ratios infer elapsed time and are diagnostic only; prospective validation remains required. |

## 11. Decision Record

### Decision 1: Three explicit scenarios before probability claims

**Context:** Existing completed-work ratios use inferred starts. No stored
out-of-sample forecast coverage establishes probability calibration.

**Decision:** Run the canonical finite-resource scheduler on deep copies for
shorter estimates, current inputs, and longer estimates plus shared disruption.
Multiply each known positive estimate by its scenario factor. Preserve missing
estimates and all other constraints. Additional loss consumes a fraction of
remaining productive capacity: new loss = 1 - (1 - effective loss) * (1 - disruption).
Apply that loss to each team in the adverse scenario, retaining roster overrides.
Scenarios may reorder through the same engine; therefore their dates are named
outcomes, not guaranteed monotonic bounds or percentiles.

**Alternatives considered:** Independent date padding would bypass resource and
dependency constraints. Automatically fitting a probability model to inferred
starts would overstate evidence quality.

**Consequences:** Results explain sensitivity now. Historical ratios remain
separate diagnostic evidence, with prospective calibration still on the roadmap.

### Decision 2: Existing context and components

**Decision:** Add Forecasts to the saved-plan view switcher, using Bootstrap
forms, cards, alerts and responsive tables. A result states the settings actually
used even if the form changes. Stale requests are ignored after context changes.
Only imported Jira captures supply diagnostics; synthetic/example snapshots are excluded.
Use existing actuals permissions and completeness rules for optional evidence;
do not create duplicate snapshot associations or rewrite agreements.

### Decision 3: Error boundaries (2026-09-07)

Saved-data decoding failures are server faults, not invalid forecast settings.
Use the existing scheduling validation classifier to preserve useful input
feedback while keeping internal decoding details out of the response. All range
settings must be finite; finite bounds already reject either infinity, while
NaN requires explicit rejection.
Account lookup failures caused by infrastructure return a generic 500 with a
retry action; only the explicit missing/expired/non-manager identity error is
an authorization failure. Do not suggest signing in again for a database outage.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|---|---|---|---|
| Manager can compare three constrained scenarios | Separate manual edits | One read-only action | Browser journey |
| Unknown work silently counted as a portfolio finish | Risk | Zero | Behavioral tests |
| Unsupported confidence labels | Risk | Zero | Tests and documentation review |

## Review Checklist

- [x] Problem, stories and Given/When/Then criteria describe manager value
- [x] Requirements distinguish behavior from implementation decisions
- [x] Access boundaries, missing evidence and failures are explicit
- [x] Scope and remaining calibration work are explicit
- [x] Behavioral, integration and browser journeys cover the first increment
