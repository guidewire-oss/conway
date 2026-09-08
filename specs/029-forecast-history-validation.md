# Forecast history validation

**Status:** In Review
**Author(s):** Conway contributors
**Date:** 2026-09-08
**Story/Ticket:** Roadmap priority 2
**Sprint/Cycle:** Comparable historical cohorts

## 1. Overview

Managers validate recorded forecasts across distinct groups of work using a
later managed capture. Results expose repeated predictions, monthly observations,
pending work and evidence gaps before managers consider changing assumptions.

## 2. Problem

Recording Beacon every week must not create four independent accuracy samples
when Beacon finishes. Selecting its most accurate prediction afterward also
overstates performance. Managers need an inspectable selection rule and evidence
counts across history without manually choosing favorable outcomes.

## 3. User Stories

### Story 1: Validate a forecasting approach across history

**As a** manager **I want** to validate comparable recorded predictions
**so that** repeated work and changed assumptions do not inflate accuracy.

### Story 2: Understand limits and investigate exceptions

**As a** manager **I want** dated groups, exclusions and original prediction links
**so that** I can investigate misses and collect missing evidence.

## 4. Acceptance Criteria

**AC 1.1:** Given several same-source predictions with matching range settings,
when history is validated against a later capture, then distinct work contributes
at most once and every repeated or excluded entry has an explanation.

**AC 1.2:** Given an earlier excluded prediction and a later successful one for
the same work, when validation runs, then the later success cannot replace the
earlier representative. Transitive overlap also counts as one work group.

**AC 1.3:** Given different settings, source configuration or records issued
after capture started, when validation runs, then these records are excluded
from the cohort with separately reported counts.

**AC 2.1:** Given completed, pending and excluded representatives, when results
appear, then overall and monthly coverage use only eligible completed entries;
zero eligible entries show unavailable. Each row links to its original record.

**AC 2.2:** Given inaccessible original evidence, an oversized history or a
failed request, when validation runs, then no partial accuracy report appears
and an actionable error offers recovery without changing saved inputs.

**AC 2.3:** Given a changed capture, prediction, identity or view during a request,
when it completes, then its result cannot replace the newer context. All controls
remain keyboard accessible and results fit a 360px viewport.

## 5. Functional Requirements

| ID | Requirement | Priority |
|---|---|---|
| FR-001 | Validation MUST reuse immutable forecasts and existing outcome completeness rules. | MUST |
| FR-002 | Repeated or overlapping captured work MUST NOT count as independent observations. | MUST |
| FR-003 | The cohort selection rule, source, settings and observation time MUST be visible. | MUST |
| FR-004 | Pending, excluded and repeated entries MUST remain distinct from eligible completed outcomes. | MUST |
| FR-005 | Managers MUST be able to investigate original records and dated cohort results. | MUST |
| FR-006 | Validation MUST preserve current plan, agreement and evidence access boundaries. | MUST |
| FR-007 | Reports MUST NOT claim calibrated probabilities, independence or recommended factors. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|---|---|---|---|
| NFR-001 | Bounded work | At most 200 records and 5000 candidate initiative entries; reject excess without partial output | Go integration/unit |
| NFR-002 | Determinism | Stable representative and row order regardless of input order | Ginkgo/Gomega |
| NFR-003 | Accessible recovery | Keyboard actions and no page overflow at 360px | Playwright |

## 7. Data Model

No new persisted entity. A validation report contains its reference prediction,
settings, outcome evidence, considered/mismatched/too-late record counts, totals,
monthly groups and traceable outcome rows. Rows add prediction ID/name, issuance
time, month and representative identity to the existing outcome fields.

## 8. API Contract

| Method | Path | Description | Request | Response |
|---|---|---|---|---|
| POST | /api/plan/{id}/predictions/{predictionId}/validation | Validate comparable history using this record's source/settings | snapshotId | Validation report |

Use existing strict bounded JSON decoding and current owner/admin authorization.
Return 400 for invalid evidence or requests, 403/404 for access violations, 422 for
history exceeding limits, and generic 500 for unreadable history/storage faults.
Original captures of every candidate record require current read access; fail
the report if access is missing rather than silently dropping those observations.

## 9. Out of Scope

- Fitting probability models, P85 dates, automated factor changes and causal claims.
- Statistical independence claims for teams sharing constraints or disruptions.
- Cross-plan pooling, selective date windows and manually dropping poor outcomes.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|---|---|---|---|
| Q1 | Does removing repeated scope establish independence? | Maintainer | This increment | No. Monthly groups remain descriptive; probability calibration needs a separate held-out model evaluation. |

## 11. Decision Record

### Decision 1: Fixed selection before outcome scoring

**Context:** Repeated forecasts and outcome-based selection inflate sample counts.

**Decision:** Use all records in the current plan, capped at 200 with an explicit
error rather than truncation. Match the chosen reference's managed source,
extraction fingerprint and exact range settings. Require the outcome capture to
match that source/configuration. Records issued at or after capture start are
reported as too late. Do not limit history to the currently loaded UI page.

Build connected components across candidate initiative entries using shared
bound epic keys and captured issue keys, including transitive overlap. Select the
earliest issuance in each component, breaking ties by record ID then initiative
name. Select before scoring: an excluded or pending representative is never
replaced by a later completed observation. Missing scope stays excluded. Names
alone do not establish shared work; reused keys conservatively remain one group.

Assess each record through AssessPrediction with current initiative inputs;
changed inputs remain excluded under spec 028. Original schedules, scope and
capture identities are never recomputed. Report every non-representative as
repeated with a link to the selected record and initiative. Count coverage as
100 * within / eligible completed representatives; zero denominator is null.
Before/within/after counts, pending and excluded representatives, and repeated
entry counts remain visible. Group representatives by UTC recording month,
including months with no eligible completions. No model is fitted to these rows.

**Alternatives considered:** Latest/best prediction selection was rejected for
outcome bias. Naive pairwise greedy suppression misses transitive overlap. Pooling
different range settings makes coverage uninterpretable. Automatic probability
labels would confuse descriptive history with demonstrated calibration.

**Consequences:** Counts are conservative and traceable. Shared team constraints,
changing operating policies and immature work can still bias comparisons; the UI
states those limits and retains pending counts. Large histories need a future
server-side cohort design; users must not delete records to fit a report.

### Decision 2: Extend the existing history workflow

Use the current prediction detail's evidence selector for Compare outcomes and
Validate history. Both actions share request ownership and a single result area;
changing evidence or action clears the preceding result. Results use Bootstrap
cards, badges and responsive layout, with optional full-entry details to avoid
crowding the first view. Preserve selection on recoverable failures and link to
contextual docs. Register this launch in feature announcements.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|---|---|---|---|
| Repeated work inflates eligible count | Manual interpretation | Zero | Transitive-overlap behavioral cases |
| Missing outcomes displayed as zero accuracy | Risk | Zero | Null-denominator and browser cases |
| Manager can trace exclusions to predictions | Manual comparison | One history workflow | Browser journey |

## Review Checklist

- [x] Problem, user stories and requirements describe manager value
- [x] Selection, access, missing data and request ownership are explicit
- [x] Calculations and inference limits are documented
- [x] Behavioral, integration and browser checks pass
- [x] User documentation and discovery accompany delivery
