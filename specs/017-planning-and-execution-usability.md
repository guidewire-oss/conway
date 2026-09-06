# Planning and execution usability

**Status:** Implemented — automated and local browser checks; user study pending
**Author(s):** Codex
**Date:** 2026-09-05
**Story/Ticket:** UX review follow-up
**Sprint/Cycle:** n/a

## 1. Overview

Make Conway's decision loop explicit: review risk, understand the constraint,
preview a change, save an agreement and review observed execution. Resolve the
findings in `docs/UX-REVIEW-2026-09-05.md` while retaining the existing engines.

## 2. Problem

Users can confuse target lateness with period overrun, absence of evidence with
healthy delivery, saved input changes with actual execution, and working-plan
edits with scenario previews. Navigation and help interrupt routine planning.

## 3. User Stories

### Story 1: Understand the current decision

As a planner I want accurate, scoped status and an answer-first workspace so
that I can identify the next decision without learning engine terminology.

### Story 2: Explore without changing an agreement

As a planner I want named scenarios, priced change previews, visible save/error
state and reversible edits so that I can understand all consequences.

### Story 3: Review execution

As a manager I want snapshot-derived actuals against the agreed baseline,
explicit coverage and recorded corrective actions so that reviews close the loop.

### Story 4: Learn and navigate independently

As a user I want consistent labels, accessible controls, resumable navigation
and accurate contextual help so that I can complete my job without a facilitator.

## 4. Acceptance Criteria

### Story 1

Given a target at week 5, finish at week 6 and a 26-week period, when status is
shown, then the target is late and the initiative still fits the period.
Given unavailable plan or quality data, when Home renders, then it reports
unknown/unavailable rather than an all-clear.

### Story 2

Given an agreed baseline, when a user duplicates a named scenario or previews
a remedy, then the original agreement and working inputs remain unchanged.
Given an applied edit, when saving fails, then visible error feedback preserves
the pending input; when it succeeds, the working-plan state reflects success.

### Story 3

Given an accessible imported snapshot and an active baseline, when execution is
reviewed, then each initiative is shown including unbound and missing-data
items; actuals identify provenance and inferred dates. Refresh never edits the
agreement. Added/removed bindings and unmapped teams are scope changes.
Given no accessible snapshot or baseline, the next setup action is shown.

### Story 4

Given a plan URL, when refreshed or revisited through browser history, then the
authorized plan and view reopen. Given help in an iframe, native section links
and Escape work. Given keyboard-only input, sorting and precise timeline edits
remain available. Narrow layouts retain navigation and help contents.

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | Status MUST separate target risk, period fit, provisional data and unknown evidence. | MUST |
| FR-002 | Home and hygiene MUST distinguish loaded aggregates from failed, missing or stale evidence and identify source scope. | MUST |
| FR-003 | Populated plans MUST start at Order unless a valid remembered view is requested; setup MUST be secondary. | MUST |
| FR-004 | Plan identity, view, selection and filters MUST be resumable through authorized URLs and browser history. | MUST |
| FR-005 | Every planning write MUST expose pending/success/error state; labels MUST distinguish settings, working inputs, proposals and agreement. | MUST |
| FR-006 | Users MUST be able to create a named independent scenario with copied inputs and no inherited agreement. | MUST |
| FR-007 | Remedies MUST offer a preview with affected commitments and explicit application to the working plan. | MUST |
| FR-008 | Timeline MUST retain independent initiative/team filters across grouping changes and show dates when the period is dated. | MUST |
| FR-009 | Timeline MUST expose a selected-item inspector and non-drag start, estimate and lane controls, with visible undo. | MUST |
| FR-010 | Execution review MUST derive per-team actuals, schedule/estimate variance, buffer risk, adherence and calibration from accessible snapshots against the active baseline. | MUST |
| FR-011 | Execution MUST include binding coverage, scope changes, data gaps, inference labels and snapshot age; missing evidence MUST NOT appear as zero variance. | MUST |
| FR-012 | Explicit epic bindings MUST be editable; suggested name matches MUST require confirmation. | MUST |
| FR-013 | Execution reviews MUST support persisted action, owner, review date and rationale, and previous decisions MUST remain available. | MUST |
| FR-014 | Refreshing execution MUST NOT mutate baseline or working inputs; Jira remains read-only. | MUST |
| FR-015 | Help MUST provide contextual manual, first-plan walkthrough, task recipes, glossary, search and narrow-screen contents from one entry. | MUST |
| FR-016 | Controls MUST use consistent visible verbs and decorative SVG icons; table sorting and dialogs MUST support keyboard access. | MUST |
| FR-017 | Documentation MUST distinguish model types, use current navigation labels and accurately describe autosave, actuals and baseline behavior. | MUST |
| FR-018 | Facilitator setup MUST explain action points, duration and learning purpose; task guidance MUST route by role and retain chosen persona. | MUST |
| FR-019 | User-facing dates and forecasts MUST state source and assumptions without presenting conditional forecasts as guaranteed commitments. | MUST |
| FR-020 | The workspace MUST support a team-focused review without expanding existing permissions or judging individual performance. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Preserve regressions | Existing Go and JS suites pass | Repository checks |
| NFR-002 | Responsive controls | Navigation and essential actions available at 360px | Browser inspection |
| NFR-003 | Accessible workflow | Keyboard sorting, focus return, non-drag editing, labeled controls | Browser and behavioral checks |
| NFR-004 | Isolation | No unauthorized plan/snapshot access; scenario and actuals do not alter baseline | Go behavioral tests |

## 7. Data Model

Execution observations are derived from snapshot issues. Bindings belong to
initiative inputs and therefore participate in baseline fingerprints. Review
decisions are append-only plan records containing action, owner, review date,
rationale, initiative, snapshot and baseline identifiers. A scenario is an
independent plan owned by its creator with copied inputs, not copied baselines.

## 8. API Contract

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| GET | /api/plan/{id}/actuals?snapshot={snapshot} | Read execution evidence | Snapshot ID | Provenance, coverage, initiative/team variance, calibration, gaps |
| POST | /api/plan/{id}/scenario | Copy an independent working plan | Name | New plan ID |
| GET/POST | /api/plan/{id}/decisions | Read/append review decisions | Decision fields | Saved decision/list |
| POST | /api/plan/{id}/schedule/remedies/preview | Compute remedy consequences without saving | Offered remedy | Before/after schedule, comparison and input fingerprint |
| POST | /api/plan/{id}/schedule/remedies/apply | Apply a reviewed remedy | Offered remedy and preview fingerprint | Updated working plan; conflict if inputs changed |

## 9. Out of Scope

Live external user research cannot be executed without participants. Usage and
delivery improvements are measured over later review cycles, not asserted by
this implementation. No Jira writeback, automatic staffing transfers, changed
permissions, dependency upgrades, publication or push.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Availability of complete issue transition history | Product owner | Next live-data review | Missing starts remain explicitly inferred/unknown; never synthesize measured actuals. |
| Q2 | User-study outcomes | Product owner | After rollout | [NEEDS CLARIFICATION] Recruit representative planners; targets are not measured results. |

## 11. Decision Record

### Decision 1: Extend the existing product and snapshot seam

Use Bootstrap and existing scheduling/snapshot storage. Add pure execution
derivation and authenticated endpoints; do not introduce a parallel integration.
Missing Jira history is an explicit limitation per item, not a reason to hide
the entire execution workflow. Explicit keys take precedence; fuzzy matches are
suggestions. Date inference and elapsed estimate comparisons carry caveats.

### Decision 2: Keep working-plan autosave and offer independent scenarios

Preserve intentional drag persistence from spec 008. A named copy is the safe
multi-step experiment; previews remain transient until explicit application.
Agreement is saved deliberately and never rewritten by execution refresh.

### Decision 3: Make the existing interface task oriented

Order becomes the default, preserving network and timeline as selectable views.
Help consolidates existing content. New controls use visible labels and a small
local SVG icon set. No new framework or downloaded dependency is required.

### Decision 4: Record decisions without duplicating Jira tasks

Store review rationale and next-action ownership against the plan, baseline and
snapshot. These are meeting decisions, not an alternative issue tracker.

### Decision 5: Label remaining-work forecasts as conditional baseline-rate proxies

For a team slice with a comparable agreed scope, a positive estimated baseline duration,
a dated baseline period and timestamped snapshot child-issue progress, estimate
remaining calendar weeks as the baseline duration multiplied by the unfinished
child-issue fraction. A conditional finish adds that remainder to the later of
the snapshot week and agreed start week. This assumes work can continue or start
at the baseline rate; it does not model newly discovered blockers or measure
effort, current capacity, throughput or calibrated delivery performance.

Return the basis beside each figure. Withhold both figures for missing evidence,
changed epic scope or unmapped team scope; preserve a specific data-gap reason.
Completed slices have zero remaining work, but their finish comes only from
resolved issue timestamps: a missing resolution date is never replaced with a
forecast. Refresh remains read-only, and forecast dates are relative to the
selected snapshot rather than the current clock.

### Decision 6: Preserve context and distinguish incomplete evidence during recovery

**Context:** Review exposed cases where a delayed response, incomplete snapshot or
shared view identifier could change what a manager believed they were seeing.

**Decision:** Persist the observed versus what-if network lens explicitly in the
route. After a delayed planning write, render the currently selected workspace.
Read-only comparisons never report a save. Plan-dependent guidance retains its
requested destination while the user chooses a plan, with a visible cancellation
action. Incomplete bound-epic scope retains observed counts but withholds whole-
scope completion, finish/risk, variance and calibration. Simulator dates use the
same duration basis as their input samples, and invalid scenarios clear previous
forecasts. Game timing guidance uses the creation defaults and bounds. Escape
returns focus to a visible invoker; fallback dialogs remain mutually exclusive.
Expandable controls expose their actual state, empty warnings stay hidden, and
imported names are escaped wherever inserted into HTML.
Preview and save validate epic bindings identically so a successful preview
cannot approve a malformed key that the matching save operation rejects.

**Alternatives considered:** Keeping a previous forecast or silently opening a
different workspace was rejected because both imply evidence the user did not
select. Treating partial scope as complete was rejected for the same reason.

**Consequences:** Recovery can require a plan choice or more evidence, but the
interface explains that requirement and retains the user's intended task.

For a rejected schedule identity, return the always-present `ScheduleFit` with
`unavailableReason`. Serialize its unavailable demand, capacity and horizon
counts as null rather than zero; consumers show the reason instead of a fit
verdict. Valid schedule responses retain their existing numeric shape.

Batch dynamic sortable-header preparation to one animation frame so a burst of
DOM updates does not repeatedly scan the entire document. Initial preparation
remains immediate, and subsequent frames prepare newly rendered headers.

Linked-source mutation completions refresh their original dialog view only when
the user has not navigated elsewhere within that dialog while awaiting the
response. Saving data does not authorize replacing a later history or preview.
This includes the callback that reloads the surrounding plan. Draft-blocked
feature actions render their pending destination and cancellation control in
place. Restored what-if routes honor the same manager access as menu navigation;
non-manager staff retain the observed network lens.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Reviewed semantic regressions | Reproduced in review | Regression checks pass | Automated checks |
| Primary-risk identification | Not measured | Under 60 seconds | Formative study |
| First demo planning loop | Not measured | Under 15 minutes | Formative study |
| Missing-data truthfulness | Misleading states | Explicit unknown/gaps | Fixtures and UI |

## Review Checklist

- Requirements and capability limits recorded before implementation.
- Regression, isolation, empty/error and keyboard scenarios required.
- Human task-success outcomes remain to be measured after implementation.
