# Forecast model evaluation

**Status:** In Review
**Author(s):** Conway contributors
**Date:** 2026-09-07
**Story/Ticket:** Roadmap priority 2
**Sprint/Cycle:** Chronological held-out evaluation

## 1. Overview

Managers can test whether an earlier portfolio history provides a useful
probability estimate for later distinct work. The workflow separates evidence
used to fit a model from evidence used to evaluate it, with visible sample gaps
and traceable predictions.

## 2. Problem

An observed coverage percentage describes work that has already finished. Using
the same outcomes both to fit and to assess a probability model overstates its
usefulness. A manager needs an honest later-work check before relying on it.

## 3. User Stories

### Story 1: Test historical learning on later work

**As a** manager **I want** to fit from earlier captured evidence and evaluate
later predictions **so that** future outcomes cannot improve the fitted model.

### Story 2: Decide whether to collect more evidence

**As a** manager **I want** sample counts, missing outcomes and plain-language
score explanations **so that** I can distinguish a useful investigation from
evidence strong enough to support a commitment.

## 4. Acceptance Criteria

**AC 1.1:** Given two chronological captures with matching provenance, when a
manager tests a model, then only outcomes available in the earlier capture fit
the model, and only predictions issued after that capture completed and before
the later capture started enter the test cohort.

**AC 1.2:** Given later outcomes change, when evaluation repeats, then the fitted
probability stays unchanged. Repeated or transitively overlapping scope cannot
contribute to both fitting and test scores.

**AC 1.3:** Given zero completed training or test observations, when evaluation
runs, then unavailable probabilities or scores remain null with a next action.
Pending, excluded, repeated and boundary-period records remain visible.

**AC 1.4:** Given current plan edits, when evaluation repeats with the same
captures, then the historical fit uses archived inputs; changed captured issue
membership still excludes affected outcomes.

**AC 2.1:** Given usable evidence, when results appear, then the event being
predicted, model formula, sample sizes, test coverage, probability error and
limitations are visible with original record links and contextual help.

**AC 2.2:** Given inaccessible captures, oversized history, invalid chronology or
service failure, when evaluation runs, then an actionable error retains input
selection and no partial report appears.

**AC 2.3:** Given the capture, prediction, identity or view changes during a
request, when it completes, then it cannot replace the newer context. Controls
are keyboard accessible and fit a 360px viewport.

## 5. Functional Requirements

| ID | Requirement | Priority |
|---|---|---|
| FR-001 | Fitting MUST exclude future observations and use only the selected earlier capture's retained evidence. | MUST |
| FR-002 | Test scores MUST exclude work represented in earlier history, including transitive overlap. | MUST |
| FR-003 | The model event, fitting rule, time boundary and denominators MUST be inspectable. | MUST |
| FR-004 | Unknown outcomes MUST remain separate from observed successes and failures. | MUST |
| FR-005 | Results MUST preserve original capture permissions, saved inputs and agreements. | MUST |
| FR-006 | The UI MUST explain that retrospective evaluation does not certify calibration or provide portfolio finish percentiles. | MUST |
| FR-007 | Managers MUST have recovery, source context and links to the original records. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|---|---|---|---|
| NFR-001 | Bounded evaluation | Existing 200-record / 5000-entry caps | Ginkgo/integration |
| NFR-002 | Reproducibility | Same archived evidence produces the same fit and scores | Ginkgo/Gomega |
| NFR-003 | Accessible recovery | Keyboard form and no overflow at 360px | Playwright |
| NFR-004 | Authorization | Current manager owner/admin and all matching original captures | Go integration |

## 7. Data Model

A read-only report contains the model version and probability; training and
test evidence; training and test counts and rows; boundary-record count; test
Brier score, signed observed-minus-predicted percentage-point gap and a fixed
50% benchmark score. Null values distinguish unavailable results from zero.
No new database entity or current-plan association is introduced. Captures are
retained evidence, not immutable model registrations: replacing their data can
change a subsequent evaluation report.

## 8. API Contract

| Method | Path | Description | Request | Response |
|---|---|---|---|---|
| POST | /api/plan/{id}/predictions/{predictionId}/evaluation | Evaluate a historical probability baseline | trainingSnapshotId, snapshotId (later test capture) | Model evaluation report |

Use the prediction workflow's strict JSON, current owner/admin authorization and
all-matching-capture access checks. Return 400 for invalid chronology or source
settings, 403/404 for access, 422 for capped history, and generic 500 for storage
faults. An empty eligible cohort is a successful report with unavailable values.

## 9. Out of Scope

- Automatically applying factors, certified calibration labels or P85 dates.
- A joint distribution for portfolio completion or treating teams as independent.
- Retrospectively claiming the model was registered or deployed before the test.
- Cross-plan pooling, hand-picked work subsets and causal claims.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|---|---|---|---|
| Q1 | What probability event can the present data support? | Maintainer | This increment | A comparable completed initiative finishing inside its recorded three-scenario envelope; not all portfolio commitments succeeding. |
| Q2 | When may a model be called calibrated? | Maintainer | Future release | No automatic threshold. This exploratory report supports investigation; prospective registered predictions and repeated independent evaluation remain necessary. |

## 11. Decision Record

### Decision 1: Transparent probability baseline with a chronological holdout

**Context:** The stored history has scenario envelopes but no declared probability
model. The next step needs a defined event and an evaluation separated in time.

**Decision:** Model version `envelope-frequency-v1` predicts the event "a completed
comparable initiative finishes inside its saved scenario envelope." Fit a single
smoothed frequency p = (training within + 1) / (training completed + 2). This
explicit weak smoothing avoids certainty from all-success or all-failure samples;
zero training completions yields null, rather than a misleading 50% model.

The earlier capture must finish strictly before the later capture starts. Fit
only predictions issued strictly before the earlier capture starts, scored with
that capture's archived outcomes. Use each prediction's archived initiative
inputs for historical assessment, so today's plan edits cannot change fitting.
This deliberately differs from current-input comparison in spec 029. All issue
scope/completeness safeguards in AssessPrediction still apply.

Training representative selection sees only earlier predictions. For the test,
select connected-scope representatives across all comparable records before the
later capture starts, before scoring outcomes. Only representatives issued
strictly after training capture completion enter the test. A later bridge may
conservatively remove test work but cannot change the earlier fit. Old pending
or excluded work never moves into the test as a later successful prediction.
Records issued during the earlier capture (including either boundary) are
reported separately and reserve their scope from later reuse.

For n eligible test completions, Brier = sum((p - y)^2) / n, with y=1 within
and y=0 before/after. Lower is better; a fixed 50% probability benchmark scores
0.25. Also show 100 * (test within / n - p) percentage points. Scores are null
without both training and test completions. Pending and excluded outcomes never
become failures. Display small-sample and shared-constraint caveats without an
arbitrary pass/fail badge: Brier measures more than calibration alone.

**Alternatives considered:** A random split leaks time information; choosing the
best later prediction repeats spec 029's selection bias; fitting finish-time
distributions to inferred starts implies unsupported effort observations.

**Consequences:** This is an inspectable retrospective test of one probability
baseline, not a deployment record or portfolio completion probability. Managers
must not search many cutoffs for a favorable result and treat it as confirmation.
Future production probability forecasts require a separately registered model
and prospective evidence. Method references (read 2026-09-07):
[chronological evaluation](https://otexts.com/fpp3/tscv.html) and
[probability calibration and Brier limits](https://scikit-learn.org/stable/modules/calibration.html).

### Decision 2: Progressive disclosure in the existing prediction detail

Add an optional Test a probability model section beneath existing comparison
actions. Reuse the later capture selector and shared result/request ownership;
the expanded section adds only an earlier training capture and one action.
Bootstrap forms, cards, alerts and responsive layouts carry the result. Lead
with sample readiness and next actions, then probability and test performance,
then expandable cohort evidence. Document the workflow and announce it once.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|---|---|---|---|
| Future outcomes change fitted probability | Untested | Zero | Leakage regression |
| Repeated work appears in fitting and test scores | Risk | Zero | Transitive-scope cases |
| Manager can identify unavailable evidence | Manual analysis | Explicit counts and next action | Browser journey |

## Review Checklist

- [x] Problem and Given/When/Then criteria describe manager value
- [x] Probability event, time boundaries and limitations are explicit
- [x] Permissions and asynchronous recovery are specified
- [x] Behavioral, integration and browser checks pass
- [x] Documentation and feature discovery accompany the workflow
