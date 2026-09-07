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

### Story 3: Learn from predictions made before outcomes

**As a** manager **I want** immutable predictions and comparable later outcomes
**so that** I can see where my planning assumptions need investigation.

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

**AC 3.1:** Given a dated comparison and managed capture, when recording a
prediction, then its inputs, issue scope and results survive reload and plan
edits. Retrying a save after a lost response does not create another record.

**AC 3.2:** Given a recorded prediction and later evidence, when comparing
outcomes, then complete comparable scope shows observed coverage and variance;
pending and excluded work remain visible with reasons and separate counts.

**AC 3.3:** Given access changes or an abandoned view, when a request completes,
then inaccessible history and results belonging to an old context are withheld.

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
| FR-009 | Managers MUST be able to record and revisit immutable predictions with source provenance and server issuance time. | MUST |
| FR-010 | Outcome assessment MUST distinguish comparable completed work, pending work and excluded scope with an explicit denominator. | MUST |
| FR-011 | Prediction history MUST retain original results after plan edits and MUST NOT replace existing agreement or snapshot associations. | MUST |

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
live inside that schedule. Prediction records archive this result, its inputs,
initial issue scope, source identity, name, author and server issuance time.
They are historical evidence, not an editable source of planning truth.

## 8. API Contract

| Method | Path | Description | Request | Response |
|---|---|---|---|---|
| POST | /api/plan/{id}/forecast | Read-only scenario computation | lowerFactor, upperFactor, disruption | fingerprint, settings, scenarios, limitations |
| GET/POST | /api/plan/{id}/predictions | List metadata or record prediction | before cursor / id (client-generated idempotency key; reuse unchanged on retry), name, fingerprint, settings, snapshotId | predictions and next cursor / immutable prediction |
| GET | /api/plan/{id}/predictions/{predictionId} | Read recorded prediction | None | Original result, inputs, scope and provenance |
| POST | /api/plan/{id}/predictions/{predictionId}/assessment | Compare later outcomes | snapshotId | rows, eligible, covered, pending, excluded, coveragePercent (null when eligible is 0), evidence |

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

The recorded-history increment measures observed scenario-envelope coverage for
one prediction at a time. Systematic validation across independent comparable
cohorts, probabilistic models and predictive calibration remain outside this
increment; observed coverage alone does not establish a calibrated probability.

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

### Decision 4: Prospective prediction history (2026-09-07)

Managers explicitly record a named prediction from a displayed comparison and
a successful managed Jira capture. The server recomputes and checks the displayed
input fingerprint, freezes the three results, inputs, issue membership, source
configuration identity and server issue time. Saving never changes an agreement.
An idempotency key makes a retried save return the original record; differing
content with that key conflicts. History is plan-scoped, paginated and immutable.
Only current manager owners or administrators can save, read or assess records.
Historical evidence still requires current access to its original capture.

The existing Forecasts view gains a Record prediction action and a separate
Prediction history section, using Bootstrap controls and tables. A manager opens
a saved prediction and chooses a later capture to compare outcomes. Reloading
retains records. Failed requests retain their inputs and offer retry. Navigating
away or changing the selected record/capture invalidates late responses.

An assessment uses the original predictions, never recomputed historical dates.
It requires the same managed source and captured extraction configuration
(site, projects, roster and team mapping, WIP mode and team field). Source names,
capture frequency and freshness preferences do not define comparability.
The later capture must start after prediction issuance. Legacy captures without
source provenance remain usable for diagnostics but cannot supply this cohort.

For each initiative, the scenario finish envelope is the minimum and maximum
finish-before-buffer date across all three known placements. The current-plan
commitment is reported separately. Complete observed scope requires bound epics,
all planned teams represented, known statuses and resolution dates no later
than capture time. Freeze child issue keys, parents, types and team assignments;
added, missing, reassigned or reparented issues exclude that initiative, as do
changed initiative inputs, missing evidence, overlapping bindings, unknown
placements, already-completed work and finishes at or before issuance.
Incomplete comparable work remains pending, not a success or failed prediction.
Use existing execution-evidence completion semantics; never infer a finish from
an issue creation timestamp. Missing or inaccessible capture data must not expose
retained private issue evidence through history.

Coverage is covered completed initiatives / eligible completed initiatives for
this single prediction, with numerator, denominator, pending and excluded counts
visible. Zero samples means unavailable, not zero percent. Show before/within/after
the envelope and finish variance from the saved current-plan forecast. Repeated
predictions are never pooled as independent samples. This is observed scenario
envelope coverage, not a calibrated probability, P85 claim or automatic factor
recommendation; systematic cohort validation remains a subsequent increment.

Acceptance journeys: record/reload/open a prediction; retry without duplicates;
reject a stale comparison or unavailable evidence; assess a later same-source
capture; report changed scope and missing timestamps separately from pending
work; reject other owners, expired roles and cross-plan record IDs; preserve
history after plan edits; ignore responses from an abandoned screen. Go behavior
and integration tests plus a Playwright manager journey cover these boundaries.

API additions: GET/POST `/api/plan/{id}/predictions` lists metadata (50 records
per page, before cursor) or records `{id, name, fingerprint, settings, snapshotId}`.
GET `/api/plan/{id}/predictions/{predictionId}` reads a record, and POST to its
`/assessment` subroute accepts `{snapshotId}` for a read-only comparison.
Responses use 400 for invalid inputs, 403/404 for access, 409 for a changed
comparison or reused key, and generic 500 for storage errors.
Inputs that cannot be serialized must never count as matching scope. Exclude
such assessments explicitly, and reject a request whose idempotency payload
cannot be encoded before attempting to record anything.

Request IDs use 128 random bits from `crypto.getRandomValues`, which is available
on plain HTTP deployments as well as secure contexts. Generate the key inside
the recording error boundary and retain it across unchanged retries. If secure
randomness is unavailable, show an actionable recording error.
Capture fingerprints include only the present extraction fields: site, projects,
rosterId, roster, wipMode, podField and teams. Canonical JSONB serialization keeps
existing fingerprints stable, including an empty configuration; operational and
unknown metadata do not affect comparability. Browser journeys restore temporary
scheduling changes in a finally block, including when an assertion fails.

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
