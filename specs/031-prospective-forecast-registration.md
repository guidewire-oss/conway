# Prospective forecast model registration

**Status:** In Review
**Author(s):** Conway contributors
**Date:** 2026-09-07
**Story/Ticket:** Roadmap priority 2
**Sprint/Cycle:** Freeze first, observe later

## 1. Overview

Managers can register a probability baseline before collecting future outcomes.
The registration freezes the fit and declares which future predictions will be
assessed, so later evidence cannot rewrite the original estimate.

## 2. Problem

Retrospective model evaluation lets a manager try different historical cutoffs.
It cannot establish that a probability was chosen before the work being tested.
A retained registration makes that distinction inspectable.

## 3. User Stories

### Story 1: Freeze a model before future work

**As a** manager **I want** to register a named model from completed history
**so that** its probability and evidence remain available after plan changes.

### Story 2: Review its future performance

**As a** manager **I want** to assess all subsequent matching distinct work
**so that** I cannot silently select only favorable predictions.

## 4. Acceptance Criteria

**AC 1.1:** Given completed comparable training observations, when registration
succeeds, then the server records its time, author, formula, probability, training
rows and original history. An unchanged retry returns the same registration.

**AC 1.2:** Given no eligible training completions, inaccessible evidence or an
oversized history, when registration is attempted, then no model is registered
and the manager receives an actionable error.

**AC 2.1:** Given a registration, when later outcomes are assessed, then only
matching predictions issued strictly after registration and before the later
capture starts contribute test observations. Earlier work reserves its scope.

**AC 2.2:** Given changed training captures or current plan inputs, when the
registered model is assessed again, then its probability and training rows remain
unchanged. Missing and repeated test work remain distinct from failed outcomes.

**AC 2.3:** Given a lost response, changed selection or navigation, when a request
finishes, then retry is safe and old results cannot replace the new context.

## 5. Functional Requirements

| ID | Requirement | Priority |
|---|---|---|
| FR-001 | A registration MUST retain its original fit and server time without an update or delete action. | MUST |
| FR-002 | Future assessment MUST use a predeclared source/settings cohort and exclude earlier overlapping work. | MUST |
| FR-003 | The workflow MUST preserve current owner/admin and original-capture access boundaries. | MUST |
| FR-004 | Managers MUST see denominators, missing evidence, model limitations and next steps. | MUST |
| FR-005 | The workflow MUST extend prediction history with contextual documentation and discovery. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|---|---|---|---|
| NFR-001 | Bounded cohorts | 200 predictions and 5000 retained matching entries; 100 models per reference | Ginkgo/Gomega |
| NFR-002 | Accessible recovery | Keyboard operable, no page overflow at 360px | Playwright |
| NFR-003 | Authorization | Persisted manager owner/admin; all retained captures | Go integration |
| NFR-004 | Retry safety | One row per plan/key | Go integration |

## 7. Data Model

ForecastRegistration stores ID, name, registered time, author, reference prediction,
training evidence, training cohort, probability and the complete matching history
known at registration. Storage retains a request hash and a plan association.
Registrations are removed only with their containing plan.

## 8. API Contract

Paths below are relative to `/api/plan/{plan}/predictions/{reference}`.

| Method | Path | Description | Request | Response |
|---|---|---|---|---|
| GET | /registrations | List accessible registrations for this reference | — | Registration summaries |
| POST | /registrations | Register a frozen model | id, name, trainingSnapshotId | Registration summary |
| POST | /registrations/{id}/assessment | Assess future matching work | snapshotId | Frozen fit and future cohort report |

Use strict JSON; 400 invalid requests/chronology/no training completions,
404 missing or inaccessible records, 403 plan/role denial, 409 conflicting key,
422 exceeded bounds and generic 500 storage failures. No partial reports.

## 9. Out of Scope

- Certified calibration, automatic model selection, P85 or portfolio finish probabilities.
- Editing registered fits, automatic changes to schedules or agreements.
- Claims of statistically independent teams or sufficient real-world validation.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|---|---|---|---|
| Q1 | When is observed evidence sufficient for production calibration claims? | Maintainer | Deployment | [NEEDS CLARIFICATION] Requires repeated prospective observations and domain review; this feature does not certify them. |

## 11. Decision Record

### Decision 1: Freeze a transparent baseline and predeclare future inclusion

**Context:** Spec 030 provides an exploratory frequency model but no registration.

**Decision:** Fit `envelope-frequency-v1` using the existing archived-input
validation and p=(within+1)/(completed+2). Require at least one eligible completed
training observation and a training capture finished strictly before server
registration time. Freeze matching prediction history, including pending and
boundary work, training rows and evidence in an immutable JSON record. The
registration explicitly applies to every subsequent prediction in this plan
with the same source, extraction fingerprint and scenario settings. No manual
test-subset picker is offered. A later model is a separate named registration;
show all registrations so unsuccessful models remain discoverable. Serialize
creation per plan and reject a new key at 100 registrations per reference;
unchanged retries still recover the original model. Enforce the 5000-entry cap
on the entire retained history, including post-training records.

Assessment combines retained history with subsequent matching predictions and
uses spec 029 connected-scope representatives before examining outcomes. Frozen
IDs cannot be replaced by subsequently changed history. Predictions issued at
registration time are conservatively reserved, never tested. Later evidence
must start strictly after registration. Fit rows are never recomputed; original
capture existence/access is rechecked without rereading their outcome data.
Brier and the fixed 50% benchmark use the existing spec 030 formula. Later
bridges can suppress overlapping test work but cannot refit the probability.

**Alternatives considered:** Attaching models manually to selected predictions
permits favorable subset selection. Recomputing old captures breaks registration.

**Consequences:** This establishes prospective timing and distinct scope, not
statistical independence or certified calibration. Retain all model attempts;
multiple attempts and incomplete observations still complicate interpretation.

### Decision 2: Progressive disclosure in prediction history

**Context:** Forecast history already offers several comparison actions.

**Decision:** Add a separate collapsed Registered models section inside a recorded
prediction. Its short form chooses an earlier capture and name; cards explain
the frozen probability, date, author and automatic future inclusion. Assessment
reuses the later capture above. Bootstrap controls and the existing context
ownership guard carry keyboard, recovery and mobile behavior.

**Consequences:** Registration remains optional and does not crowd the main
scenario comparison. A contextual guide explains the next capture cycle.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|---|---|---|---|
| Later evidence changes registered probability | No registration | Zero | Mutation regression |
| Earlier overlapping work enters prospective score | Untested | Zero | Chronology/scope cases |
| Manager can register, retry and assess later work | Missing | Complete journey | Browser acceptance |

## Review Checklist

- [x] Problem, behavior and limits specified before implementation
- [x] Behavioral, integration and browser checks pass
- [x] Documentation and feature discovery accompany the workflow
