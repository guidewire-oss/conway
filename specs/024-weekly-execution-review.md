# Weekly execution review and action follow-up

**Status:** Done
**Author(s):** Project maintainers
**Date:** 2026-09-06
**Story/Ticket:** Feature strategy F01, with a lightweight outcome note
**Sprint/Cycle:** Planning and execution follow-up

---

## 1. Overview

Managers can prepare, complete and reopen a weekly review inside a plan's existing
Review execution view. The review separates delivery exceptions from missing
evidence, compares its context with the last completed review, and follows actions
through explicit states with resolution evidence and preserved history.

---

## 2. Problem

An execution snapshot explains one observation but does not record what a manager
reviewed or whether a previously agreed action was completed. Combining missing
evidence with delivery exceptions can make a team appear late when its progress is
simply unknown. For example, Atlas can have an overdue follow-up while Beacon has
no captured epic; a review must distinguish those two conversations.

---

## 3. User Stories

### Story 1: Prepare an evidence-aware review

**As a** manager
**I want** an agenda for one plan, review date and timezone
**So that** I can distinguish delivery exceptions, overdue actions and data gaps.

### Story 2: Follow actions to an evidenced conclusion

**As a** manager
**I want** explicit action status and an append-only transition history
**So that** I can explain which commitments remain open and why others were closed.

### Story 3: Preserve and revisit a completed review

**As a** manager
**I want** a saved review summary with its original evidence context
**So that** I can compare the next review and share the same authorized record.

---

## 4. Acceptance Criteria

### Story 1: Prepare an evidence-aware review

**AC 1.1: Separate evidence gaps from delivery exceptions**
> Given Atlas has measured agreement divergence and Beacon lacks bound epic evidence,
> when a manager previews a review,
> then Atlas appears in delivery exceptions and Beacon's missing evidence appears
> in data gaps without manufacturing a completion percentage or late finish.

**AC 1.2: Explicit comparison context**
> Given a prior completed review,
> when the manager prepares another review,
> then both snapshot and agreement contexts are identified; unchanged capture IDs
> are labeled no new capture, and changed agreement or scope contexts qualify
> comparisons rather than claiming directly comparable progress.

**AC 1.3: Manual review without a snapshot**
> Given no accessible snapshot is selected,
> when the manager prepares and completes a manual review,
> then actions and qualitative outcomes remain usable, the evidence gap is explicit,
> and measured progress, forecast movement and delivery improvement stay unknown.

**AC 1.4: Review-relative overdue actions**
> Given a valid chosen date and IANA timezone,
> when an action's due review date precedes that review date and its state is open
> or in_progress,
> then it is overdue; equal-date actions and resolved or superseded actions are not.

### Story 2: Follow actions to an evidenced conclusion

**AC 2.1: Legacy compatibility**
> Given an execution decision created before action states existed,
> when it is listed through the existing decisions API,
> then its original fields remain intact and it has open status and version one.

**AC 2.2: Evidenced transitions**
> Given a current action version,
> when a manager resolves or supersedes it,
> then nonblank evidence is required and the successful transition records its
> author, timestamp, previous state and new state without rewriting the decision.

**AC 2.3: Concurrent action edits**
> Given two clients read the same action version,
> when one transitions it and the other submits its stale version,
> then the stale request receives a conflict and cannot add a transition or overwrite state.

**AC 2.4: Reopen and plan isolation**
> Given a resolved action,
> when an authorized manager reopens it,
> then the earlier resolution evidence remains in history; another plan or an
> unauthorized account cannot read or transition that action.

### Story 3: Preserve and revisit a completed review

**AC 3.1: Immutable completion**
> Given a successfully reviewed preview,
> when the manager completes the review,
> then the stored agenda, context, action states, filters and outcome are immutable;
> later plan edits, snapshots, agreement activation or action transitions cannot
> change the reopened summary or any saved agreement.

**AC 3.2: Refuse changed context**
> Given the plan, active agreement, selected evidence, action versions or preceding
> completed review changed after preview,
> when the manager attempts completion with the old fingerprint,
> then completion is refused with a conflict requiring a refreshed preview.

**AC 3.3: Authorized sharing and accessible use**
> Given a completed review,
> when a manager follows its plan review link or uses the controls by keyboard on
> a narrow viewport,
> then the same summary is reachable within Review execution; another account
> gains no access merely by possessing the link.

**AC 3.4: Failed saves preserve work**
> Given an outcome note or transition evidence has been entered,
> when its save fails or conflicts,
> then the entered text remains available, the error is visible and no successful
> completion or resolution is claimed.

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | Review preparation and history MUST live within the existing one-plan execution view. | MUST |
| FR-002 | The agenda MUST separate delivery exceptions, data gaps and action follow-up with explainable reasons and supporting initiative references. | MUST |
| FR-003 | Comparison MUST identify the selected and preceding completed review contexts, distinguishing absent evidence, unchanged capture and changed agreement or scope. | MUST |
| FR-004 | A manager MUST be able to complete a manual review without a snapshot; unknown measures MUST remain unknown. | MUST |
| FR-005 | Actions MUST support open, in_progress, resolved and superseded states, preserving original decisions and all successful transitions. | MUST |
| FR-006 | Resolving or superseding an action MUST require evidence; stale versions MUST be refused without partial writes. | MUST |
| FR-007 | Overdue status MUST use the chosen review date and timezone, excluding resolved and superseded actions. | MUST |
| FR-008 | Completed summaries MUST preserve observed agenda, context, action state, selected filters and qualitative outcome without later rewriting. | MUST |
| FR-009 | Completion MUST refuse a changed reviewed context and MUST preserve user input on failure. | MUST |
| FR-010 | Existing decisions clients MUST remain usable; old decisions MUST default to open. | MUST |
| FR-011 | Review links and action history MUST enforce the plan's existing account and role authorization. | MUST |
| FR-012 | The summary MAY record a next checkpoint alongside its qualitative outcome note; this MUST NOT be presented as a complete initiative outcome model. | MAY |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | Atomic concurrency handling | One successful version transition per expected version; no partial review completion | Concurrent database tests |
| NFR-002 | Accessible controls | Keyboard operation, visible labels, announced errors, no page overflow at 360px | Real browser acceptance |
| NFR-003 | Plan isolation | No review/action disclosure or write outside authorized plan | API integration tests |
| NFR-004 | Evidence integrity | Completed JSON and agreement content unchanged after later writes | Database integration tests |
| NFR-005 | Safe rendering | All stored user text renders as text | Browser or DOM markup regression |

---

## 7. Data Model

### Entities

**ExecutionAction** extends an existing ExecutionDecision:
- Existing id, planId, action, owner, reviewDate, rationale, initiative, snapshotId,
  baselineId, createdBy and createdAt remain unchanged.
- status: open | in_progress | resolved | superseded.
- version: positive integer, initially one.

**ActionTransition**:
- id, decisionId, planId: immutable identifiers.
- fromStatus, status, version: transition and resulting version.
- evidence: text; required for resolved and superseded.
- createdBy, createdAt: authenticated actor and server timestamp.

**ReviewContext**:
- reviewDate: YYYY-MM-DD civil date; timezone: validated IANA zone name.
- planId, planFingerprint: reviewed working-plan identity.
- snapshot: nullable id/name/source/capturedAt metadata; absence means manual review.
- baseline: nullable active agreement id/name/period context.
- previousReview: nullable preceding review id/date/snapshot/baseline/plan fingerprint.
- comparisonLabel: explicit comparison limits or no-new-capture explanation.

**ReviewSummary**:
- context: ReviewContext.
- agenda: delivery and gaps arrays of entries with kind, reason, optional initiative
  and team, and available supporting values where comparison is justified.
- actions: action state/version observations with overdue flags for this review date.
- counts: delivery, gaps, openActions, overdueActions and resolvedActions counts.
- filters: optional team and initiative selections retained as presentation context.
- evidence: optional existing ExecutionActuals used to preserve comparable observed
  values for a subsequent review; absent evidence remains absent.
- No evidence-derived percentage or movement is synthesized when evidence is missing.

**ExecutionReview**:
- id, planId, createdBy, createdAt: immutable identity and completion provenance.
- reviewDate, timezone, snapshotId, baselineId: frozen selected context references.
- preview: immutable ReviewSummary plus the reviewed fingerprint.
- outcomeNote: required nonblank conclusion, at most 10000 characters.
- nextCheckpoint: optional YYYY-MM-DD date.

### Relationships

- One plan has many decisions/actions and completed reviews.
- One action has an ordered append-only transition history.
- One review references its preceding completed review and captures action versions;
  it does not own or rewrite those actions, snapshots or agreements.

---

## 8. API Contract

All paths are under the existing authorized `/api/plan/{planId}` boundary.
All new mutation bodies reject unknown fields, malformed values and trailing JSON.

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| GET/POST | /decisions | Existing list/create, retaining legacy fields | Existing decision body | Existing envelope/object plus status/version |
| GET | /decisions/{id}/transitions | Read append-only action history | — | {transitions: ActionTransition[]} |
| POST | /decisions/{id}/transitions | Append an optimistic transition | {expectedVersion, status, evidence} | Updated ExecutionAction |
| POST | /reviews/preview | Prepare server-derived agenda | {reviewDate, timezone, snapshotId?, filters?} | ReviewSummary plus fingerprint |
| POST | /reviews | Complete the reviewed context | Preview inputs plus {expectedFingerprint, outcomeNote, nextCheckpoint?} | ExecutionReview |
| GET | /reviews | List completed reviews, newest completion first | — | {reviews: ExecutionReview[]} |
| GET | /reviews/{id} | Reopen immutable summary | — | ExecutionReview |

Errors use 400 for invalid input, 401/403 for authentication/role requirements,
404 for inaccessible or cross-plan references, and 409 for version/context conflict.
New successful mutations return a JSON object; clients use the returned version.
No PATCH or DELETE can change a completed review or transition history.

---

## 9. Out of Scope

- Notifications, email, chat delivery or writes to Jira.
- A new top-level review navigation area or cross-plan review aggregation.
- Persisted collaborative draft editing, recurring scheduling or approval workflows.
- A new forecasting model, urgency score or inferred progress between snapshots.
- Full outcome or minimum-scope initiative modeling; this increment records qualitative review fields only.
- Public share tokens that broaden the plan's access permissions.

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should reviews later support collaborative drafts and amendments? | Maintainers | After first pilot | Deferred; this increment completes one immutable record and reopens it read-only. |

---

## 11. Decision Record

### Decision 1: Reuse the existing execution workflow and evidence

**Context:** Feature strategy F01 extends existing review decisions; parallel
screens or invented progress would obscure the current evidence limitations.

**Decision:** Keep reviews within Review execution. Reuse the existing actuals
derivation with the selected accessible snapshot and active agreement. Classify
measured late/at-risk or agreement divergence and scope changes as delivery
exceptions, while preserving derivation gaps in a separate section. Compare only
available matching measures; changed agreement or working scope is an explicit
comparability limitation. A reused capture says no new capture. A manual review
retains actions and qualitative notes with a no-snapshot data gap.

**Alternatives considered:** A new weekly dashboard or opaque priority score —
rejected because neither is required to make the existing review actionable.

**Consequences:** This increment does not infer new blockers or movement that
the existing actuals and stored review observations cannot establish.

### Decision 2: Preserve decisions and append state transitions

**Context:** Decisions already have immutable action text, owner, due review date
and context references. Existing clients must keep creating and reading them.

**Decision:** Keep decision IDs as action IDs. Add current state/version and an
append-only event history, atomically updated using expectedVersion. Existing
records default to open/version one. Any different state among the four states
is allowed, including reopening, but resolved/superseded require trimmed evidence.
Submitting the same status is invalid rather than silently appending a duplicate.
Transitions do not rewrite original action fields; revising an action can use a
new decision and supersede the old one with an explanation.

**Alternatives considered:** Replacing decisions with a second action catalog —
rejected because it duplicates identity and risks losing existing records.

**Consequences:** Resolution remains auditable; owner/due-date editing is deferred.
Overdue means an unresolved action's valid due date is strictly before the chosen
review civil date. The timezone validates and labels that civil-date context;
server wall-clock time does not change a saved overdue result.

### Decision 3: Preview and atomically complete an immutable review

**Context:** Managers need to save and reopen a summary without a large draft
collaboration model or silently capturing a different context from the preview.

**Decision:** POST preview computes the summary and fingerprint; explicit POST
completion recomputes under a consistent plan/action/review context and refuses
a mismatched expectedFingerprint. The fingerprint includes relevant working
inputs, active agreement, selected snapshot evidence, action versions, previous
completed review identity, review date/timezone and filters. Completion persists
one immutable record and server-derived summary; no client-supplied agenda is
trusted. The preceding review is the most recently completed record in the same
plan, ordered by a monotonic database completion sequence assigned under the plan
lock; equal timestamps or random ID order cannot leave an older record current. Reopen means view
the original summary, not resume editing. Context changes require a new preview.

**Alternatives considered:** Mutable saved drafts — deferred to avoid adding
collaborative editing and amendment semantics to the first increment.

**Consequences:** Failed completion retains local form values. The outcome note
and optional next checkpoint are qualitative review annotations, not measurements
or claims that delivery improved. Review links retain existing plan authorization.
The frozen filters can seed a subsequent review, but changing snapshot or filters
requires an explicit refreshed preview; background refresh must not replace a
review being considered or its form values.

### Decision 4: Independent and deterministic acceptance

**Context:** The factory separates product implementation from test authorship.

**Decision:** Use Ginkgo/Gomega for pure agenda/state rules and isolated PostgreSQL
integration, plus the existing real-server Playwright pattern for the in-plan
manual review/action/summary workflow. Browser fixtures and provider seams remain
test-only. Use existing toolchain and dependencies; no external service is required.

**Alternatives considered:** Only source-string assertions — rejected for state,
concurrency and saved-summary behavior that requires observable execution.

**Consequences:** Tests cover stale versions, immutable completion, authorization,
manual evidence and keyboard/narrow-viewport interactions with generic data.

---

### Decision 5: Filter visible exceptions without discarding source evidence

**Context:** A team filter must retain assigned work with missing measurements,
while a completion increase is not itself a delivery exception.

**Decision:** Apply team and initiative filters to agenda entries and
initiative-linked actions using current plan membership, including manual and
untracked work. Retain plan-wide gaps and actions when a filter is selected.
Counts describe the filtered agenda/action set, and the summary displays its
filters. Keep original actuals evidence in the frozen record for subsequent
comparison. A positive issue-count completion movement is not added to delivery
exceptions; a decrease can be an explained regression with an issue-scope caveat.
Unchanged snapshot IDs disclose no new capture even if agreement or scope also
changed. Unknown provenance is labeled unknown; only known baseline/template
sources are positively labeled synthetic.
Different snapshot IDs do not establish chronological advancement: when both
capture timestamps are known and the selected timestamp is equal to or earlier
than the preceding review's capture, label it not newer and withhold directional
progress or regression claims between those captures.

**Alternatives considered:** Filtering only measured team slices — rejected
because it hides assigned work precisely when its evidence is missing.

**Consequences:** Filtered counts are not whole-plan totals. These rules provide
explicit review context without introducing a new progress or priority model.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Ability to revisit last review and explain an action state | No completed review object | Complete the task with preserved evidence and context | Pilot observation and browser acceptance |
| Resolutions with supporting evidence | Not represented | Every resolved transition requires evidence | API invariants |
| Review preparation time | Not yet measured | Establish baseline and observe change over four reviews | Voluntary pilot timing; no claimed improvement before measurement |

---

## Review Checklist

- [x] Problem is clearly stated and justified
- [x] User stories represent real user value
- [x] Acceptance criteria are in Given/When/Then format
- [x] Edge cases and error scenarios are covered
- [x] Requirements use MUST/SHOULD/MAY language
- [x] Non-functional requirements have measurable thresholds
- [x] Out of Scope is explicit
- [x] Open questions are marked, owned, and time-bound
- [x] No implementation details in the requirements (WHAT/WHY, not HOW)
- [x] AI can read this spec (markdown, in the repo)
