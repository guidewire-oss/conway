# Team ready-work queue and release decisions

**Status:** Implemented
**Author(s):** Project maintainers
**Date:** 2026-09-06
**Story/Ticket:** Feature strategy F02
**Sprint/Cycle:** Planning and execution follow-up

---

## 1. Overview

Next work gives a manager a team-specific queue within an existing plan, using
the schedule and ordering already in force. It distinguishes reported carryover,
work eligible for a release decision, waiting work and deliberate deferrals,
with an explicit planning week, operational checklist and preserved decision history.

---

## 2. Problem

A planned reservation is not proof that a team started work or that its operational
prerequisites are accepted. For example, Team A can have a free lane while Atlas
waits for Team B, or a past planned start can lack any confirmation of actual work.
A manager needs an explained next decision without a second scheduler inventing
capacity, moving dates or interpreting empty evidence as completed delivery.

---

## 3. User Stories

### Story 1: Understand the team's next decision

**As a** manager
**I want** a team queue for an explicit week with reasons and schedule links
**So that** I can distinguish readiness from forecast placement and unknown status.

### Story 2: Confirm operational prerequisites

**As a** manager
**I want** a small checklist with accountable owners and evidence
**So that** a scheduled opportunity is not mistaken for permission to begin unready work.

### Story 3: Record and revisit a release or deferral

**As a** manager
**I want** preserved release, defer and reconsider decisions
**So that** I can explain the team's choices and revisit them without rewriting history.

---

## 4. Acceptance Criteria

### Story 1: Understand the team's next decision

**AC 1.1: Accepted schedule, retained scope**
> Given a saved plan and selected team,
> when Next work loads for a chosen week,
> then all that team's assigned initiatives are represented, including held and
> unestimated work, using the current ordering in force and its schedule reasons.

**AC 1.2: Forecasts do not become actual starts**
> Given an initiative's planned start precedes the chosen week without a declared
> carryover state,
> when its queue row is shown,
> then it is Waiting with an explanation to verify status or replan, never
> automatically In progress or completed.

**AC 1.3: Qualified carryover and release window**
> Given an initiative is explicitly marked InFlight,
> when it appears under In progress,
> then the label says reported initiative carryover and does not assert confirmed
> team activity; otherwise readiness requires the selected team's planned start
> to equal the chosen week and all required gates to hold.

**AC 1.4: Capacity and whole-chain constraints**
> Given an eligible start week but missing estimates, held downstream work,
> a calendar gap, unavailable capacity or another blocking scheduler condition,
> when the manager reviews the row,
> then it remains Waiting with a specific reason and no invented earlier slot.

**AC 1.5: Acceptance checkpoints and finished scope**
> Given an assigned zero-effort milestone or explicitly fully progressed carryover,
> when Next work is rendered,
> then the milestone is labeled an acceptance checkpoint without a fabricated
> work week, and the fully progressed scope remains inspectable as declared
> complete context rather than a new release opportunity.

### Story 2: Confirm operational prerequisites

**AC 2.1: Required checklist evidence**
> Given an initiative/team pair,
> when a manager records operational readiness,
> then each checked required item has an owner, evidence and server confirmation
> time; unchecked items remain explicit blockers, and partial progress can be saved.

**AC 2.2: Separate operational and scheduling gates**
> Given the operational checklist is complete but KitPct is below the configured KitGate,
> when the queue is computed or release attempted,
> then the scheduler's gate still holds and no checklist write changes KitPct,
> the working schedule, existing planning inputs or a saved agreement.

**AC 2.3: Reconfirm changed context**
> Given recorded checklist evidence,
> when planning inputs or the selected operational week differ from its confirmation,
> then the earlier evidence remains visible as historical context but cannot
> silently authorize a release in the changed context.

### Story 3: Record and revisit a release or deferral

**AC 3.1: Release is an evidenced decision**
> Given a currently Ready to pull row with a current queue fingerprint,
> when a manager records release with an owner and evidence,
> then an immutable decision is appended; it records authorization, not an
> observed start, and cannot change Jira, schedule placement or an agreement.

**AC 3.2: Deliberate deferral can be reconsidered**
> Given eligible assigned work that is deliberately deferred,
> when a manager later records reconsideration,
> then the earlier deferral remains in history and the queue re-evaluates all
> current gates rather than automatically releasing the work.

**AC 3.3: Stale or unauthorized decisions fail atomically**
> Given a changed plan, readiness record or intervening decision,
> when a client submits an old fingerprint or another plan's item,
> then the operation is refused without appending history or changing state.

**AC 3.4: Usable recovery and navigation**
> Given a save fails or conflicts,
> when the manager returns to the form,
> then entered evidence is retained and the error explains how to refresh;
> keyboard and 360px layouts allow queue review, history and existing timeline links.

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | Next work MUST remain within one saved plan and use its current schedule and ordering in force. | MUST |
| FR-002 | The queue MUST identify team, as-of planning week, period and evidence basis; a forecast MUST NOT be represented as an observed start. | MUST |
| FR-003 | Assigned held, unestimated, milestone and completed context MUST remain inspectable rather than silently disappearing. | MUST |
| FR-004 | Groups MUST distinguish In progress, Ready to pull, Waiting and Deferred, with specific reasons and responsible owner where known. | MUST |
| FR-005 | Ready work MUST satisfy both existing scheduling constraints and current operational checklist requirements. | MUST |
| FR-006 | Checklist records MUST retain item state, owner, evidence, actor, time and reviewed context without rewriting prior confirmations. | MUST |
| FR-007 | Release, defer and reconsider decisions MUST be append-only, require owner/evidence and refuse stale context atomically. | MUST |
| FR-008 | Readiness and release writes MUST NOT alter KitPct, schedule inputs, observations, Jira or saved agreements. | MUST |
| FR-009 | Deferrals MUST have a visible reconsideration path that rechecks readiness. | MUST |
| FR-010 | Access MUST follow existing manager/admin plan authorization and reject cross-plan references. | MUST |
| FR-011 | Failed operations MUST retain entered evidence and show a recoverable error. | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | Authoritative placement | No queue-specific rescheduling or fabricated lane weeks | Pure schedule/queue parity tests |
| NFR-002 | Atomic history | One successful write from a reviewed context; stale competitors refused | Concurrent database tests |
| NFR-003 | Authorization and integrity | No cross-plan disclosure/write; agreements unchanged | API/database integration |
| NFR-004 | Accessibility | Keyboard controls, visible labels, announced errors, no page overflow at 360px | Real browser acceptance |
| NFR-005 | Safe rendering | User-controlled names/evidence displayed as text | DOM/browser tests |

---

## 7. Data Model

### Entities

**ReadyQueueContext**:
- planId, planFingerprint: saved planning input identity.
- team, asOfWeek: selected existing team and nonnegative integer week.
- periodStart, horizonWeeks, acceptedOrdering, basis: explicit schedule context.
- The as-of week is a chosen planning coordinate, not a live observation timestamp.

**ReadyQueueItem**:
- initiative, team: existing input membership identity.
- state: in_progress | ready | waiting | deferred | complete.
- kind: work | milestone; milestone label is Acceptance checkpoint.
- plannedStartWeek, plannedFinishWeek, earliestFeasibleWeek: nullable schedule
  placement values; absence is not week zero.
- reasons: entries with code, message and owner (blank when unassigned).
- checklist: entries with key, label, required, checked, owner and evidence.
- confirmation, lastDecision: nullable current/historical readiness records.
- confirmationCurrent, releaseCurrent: server-derived flags identifying whether
  the stored evidence/authorization matches current planning inputs and week.
- canRelease: server-derived boolean; a recorded release does not mean work started.
- Queue counts contain total, inProgress, ready, waiting, deferred and complete.
- Item identity remains its existing initiative/team pair; this does not create
  new planning identity or an independent ordering.

**ReadyConfirmation**:
- id, planId, team, initiative, asOfWeek, planFingerprint.
- checks: fixed required keys scope_ready, dependencies_accepted, team_available.
- Each check has checked, owner and evidence. Checked items require nonblank
  owner/evidence; unchecked items are recorded without asserting confirmation.
- createdBy, createdAt: server-confirmed provenance.

**ReleaseDecision**:
- id, planId, team, initiative, asOfWeek, planFingerprint.
- decision: release | defer | reconsider.
- owner, evidence: required explanation and accountable owner.
- createdBy, createdAt: immutable server provenance.

### Relationships

- One plan has many initiative/team confirmations and decisions.
- Readiness is operational metadata attached to current source membership; it
  is not a replacement initiative, team, schedule or agreement model.
- History survives later input edits/removal and remains scoped to its plan.

---

## 8. API Contract

Paths are relative to the existing authorized `/api/plan/{planId}` boundary.

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| GET | /ready-queue | Derive the selected team queue | Query team, asOfWeek | {fingerprint,context,items,counts} |
| POST | /ready-queue/confirmations | Append operational checklist state | {team,initiative,asOfWeek,expectedFingerprint,checks} | ReadyConfirmation |
| POST | /ready-queue/decisions | Append a release/defer/reconsider decision | {team,initiative,asOfWeek,expectedFingerprint,decision,owner,evidence} | ReleaseDecision |
| GET | /ready-queue/history | Read retained history for an item | Query team, initiative | {confirmations,decisions} |

Successful mutations return 200 JSON. New mutations reject unknown fields,
trailing JSON, duplicate/unknown checklist keys and invalid dates/weeks/names.
Errors use 400 for invalid content or an ineligible release, 401/403 for account
authorization, 404 for inaccessible/missing references and 409 for stale context.
For history, an existing initiative/team assignment with no records returns empty
arrays. A removed assignment with retained records remains readable. A pair with
neither current membership nor retained records returns 404; blank coordinates
remain malformed requests (400).
Requests cannot select their actor, timestamps, derived state or scheduling outcome.

---

## 9. Out of Scope

- Another scheduling algorithm, moving work into an idle lane, or automatic reprioritization.
- New snapshot ingestion/integration, declared team-start events or progress editing.
- Automatic Jira updates, notifications or messaging.
- Cross-plan capacity commitments or dependency inboxes.
- Replacing the existing numeric readiness percentage with checklist arithmetic.
- Claiming operational readiness proves actual resource availability or delivery.

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should a later increment include explicit team start/completion observations? | Maintainers | After first team pilot | Deferred; this increment distinguishes existing declared carryover and release authorization. |

---

## 11. Decision Record

### Decision 1: Conservative release opportunities from the existing schedule

**Context:** The scheduler already enforces ordering, calendars, dependencies,
WIP, leads and physical tracks. Past planned work does not establish actual state.

**Decision:** Recompute saved inputs through the existing accepted-order scheduler;
when no ordering is saved, the effective default and displayed label are stated
priority, matching the scheduler. Do not build a parallel planner. A new work item may become ready only at its
actual scheduled team start week, with estimated, nonprovisional whole-chain
placement and a valid positive-work reservation. Consecutive phase growth is
allowed; interrupted work reservations require review/replanning. Administrative
finish holds do not fabricate occupied work. Future starts are Waiting for their
scheduled window; past starts are Waiting for status verification/replanning.
Missing/held placement has null dates and the scheduler's reason. An unavailable
schedule cannot authorize release. The chosen week must be within the plan's
visible horizon; moving beyond it requires extending/reviewing planning inputs.

**Alternatives considered:** Pull into any apparently free lane — rejected because
it can bypass whole-chain reservations and operational uncertainty.

**Consequences:** This queue is deliberately conservative. Its earliest feasible
week is the schedule's existing placement, never a newly optimized promise.

An explicitly requested team absent from the plan remains an invalid choice:
show its name in a notice and select an empty Choose a team from this plan option.
Do not silently choose a different team or request a queue for that invalid value.
Only a successful queue response persists the chosen team/week to navigation;
a failed request preserves the last accepted route. Refreshing the same context
does not create another history entry. Team changes in Review execution also
update the plan's shared team filter so Next work and timeline retain the choice.

### Decision 2: Keep operational evidence separate from numeric full-kit readiness

**Context:** KitPct describes readiness at period start, and KitGate already
constrains new work. A checklist cannot silently rewrite that planning assumption.

**Decision:** Use three required operational checks: scope_ready,
dependencies_accepted and team_available. Each checked item requires an owner and
evidence. Partial confirmations are allowed and leave missing checks as blockers.
All checks must be confirmed for the current complete planning fingerprint and
selected as-of week before release; team availability is week-specific. Changes
require reconfirmation while preserving earlier evidence. Confirmation does not
change KitPct, bypass KitGate or override predecessor/calendars/capacity decisions.
Owner absence is displayed as unassigned rather than inferred from a random lead.

**Alternatives considered:** Deriving KitPct from checked-item counts — rejected
because these three operational questions do not represent the original model's
full-kit effort or readiness semantics.

**Consequences:** Full-plan scope guards may require reconfirmation after edits
elsewhere in the plan; the UI must explain that the reviewed whole-chain context changed.

### Decision 3: Preserve decisions without manufacturing execution state

**Context:** Release is a manager's authorization, while carryover is an existing
initiative-level declaration. Neither proves a selected team has started work.

**Decision:** Keep In progress exclusively for existing InFlight declarations,
explicitly labeled reported initiative carryover, not confirmed team activity.
InFlight with ProgressPct at one is retained as declared complete context. For
new work, release/defer/reconsider append immutable decisions with owner/evidence.
Release requires current canRelease and moves the item to Waiting with the
reason Release recorded; start remains unconfirmed, not an observed-start state.
Defer excludes carryover/complete items; reconsider is allowed only after defer and
clears the latest deliberate deferral and reevaluates all gates. Repeated release
for an unchanged current released context is refused rather than adding duplicates.
A deferral persists until explicit reconsideration, even after an input change,
while a historical release cannot authorize changed scope. Zero-effort milestones
are acceptance checkpoints: they may be released only at their scheduled week
with accepted prerequisites, and consume no invented lane or week.

The scheduler's provisional marker alone does not reject an explicitly estimated
zero-effort checkpoint. This exception applies only when every flagged unestimated
team is explicitly Estimated with zero Weeks, and the selected initiative and its
ancestors have valid roster membership, known estimates, and resolved, acyclic
team and initiative dependencies. Missing positive estimates or unresolved/cyclic
dependencies still block release; all other checklist and scheduled-week gates remain.

**Alternatives considered:** Treating release as started, or adding team progress
events immediately — rejected/deferred to keep this increment evidence-honest and bounded.

**Consequences:** Completed and milestone context remain visible outside ordinary
positive-work recommendations. This increment never infers Jira progress.

### Decision 4: Atomic scope guards and deterministic tests

**Context:** A checklist or decision must match the queue the manager reviewed.

**Decision:** The queue fingerprint covers plan identity, complete planning inputs
(including site working hours even when derived team rows do not change), selected
team/week and current readiness/decision state. Persisted confirmation and release
freshness uses the same complete plan scope. Identical plans have distinct tokens.
Append writes lock the plan and
revalidate the same scope and eligibility atomically; no stale write may append
history. A monotonic persisted order selects the latest event even within one
timestamp. Its database column is authoritative: immutable event JSON omits
eventOrder, and API responses derive that field from the persisted column.
Use pure Go queue tests, Ginkgo/Gomega isolated database integration,
JS rendering tests and the existing real-server Playwright harness with generic
fixtures. No new dependencies or production fixture endpoints are introduced.

**Alternatives considered:** Timestamp-only or unchecked last-write-wins state —
rejected because same-second concurrent writes can silently authorize stale work.

**Consequences:** Failed forms retain evidence and require a refreshed queue;
saved agreements and planning inputs remain unchanged by operational writes.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Explain why a team should wait or may request release | Requires reading several schedule views | Complete the queue task with a stated reason and source context | Team pilot and browser acceptance |
| Releases supported by current checklist evidence | Not recorded | Every accepted release has current evidence and owner | API/database invariants |
| Avoidable blocked starts | Not measured | Establish baseline before claiming improvement | Voluntary team pilot over several reviews |

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
