# Evidence-linked planning assistant

**Status:** Implemented; release review pending
**Author(s):** Anoop Gopalakrishnan, Codex
**Date:** 2026-09-07
**Story/Ticket:** Product roadmap priority 1
**Sprint/Cycle:** First assistant increment

---

## 1. Overview

Help managers explain the selected saved plan, inspect changes against its
agreement and prepare an execution-review agenda using authorized Conway evidence.
Answers show their context and source links and distinguish modeled placement,
recorded observations and unknowns. The first increment is read-only.

## 2. Problem

A manager currently crosses the timeline, agreement comparison and review screens
to answer a question such as why Atlas cannot start Beacon integration. An
assistant that invents a scheduling reason or silently switches snapshots would
be worse than that manual investigation. Explanations must retain the existing
calculation and permission boundaries.

## 3. User Stories

### Story 1: Explain the selected schedule

**As a** manager **I want** an explanation of a selected initiative's placement
**So that** I can inspect its actual constraints and follow the relevant source.

### Story 2: Review changes and attention items

**As a** manager **I want** agreement changes and a review agenda for explicit
evidence and date context **So that** I can prepare decisions without duplicating
review records or mistaking missing observations for successful delivery.

### Story 3: Recover and retain context

**As a** manager **I want** visible scope, safe failures and contextual entry points
**So that** a late response, unavailable service or changed plan cannot mislead me.

## 4. Acceptance Criteria

### Story 1: Explain the selected schedule

**AC 1.1: Authoritative explanation**
> Given an accessible saved plan and initiative,
> when a manager asks to explain its schedule,
> then the answer uses the existing ordering and scheduler results, identifies
> modeled placement and holds, and links to the selected initiative and timeline.

**AC 1.2: Unknowns remain unknown**
> Given missing estimates, incomplete placement or provisional assumptions,
> when an answer is prepared,
> then it retains those limitations and never invents a date or observed start.

### Story 2: Review changes and attention items

**AC 2.1: Agreement scope**
> Given an active agreement,
> when changes are requested,
> then the current plan is compared with that agreement using the existing
> comparison function, retaining unplaced work and changed calendar context;
> without an agreement the answer identifies the missing comparison basis.

**AC 2.2: Evidence-aware agenda**
> Given a selected snapshot or explicit manual review, review date and timezone,
> when a review agenda is requested,
> then it uses the existing review preparation, separates delivery exceptions,
> actions and evidence gaps, and creates no completed review or action.

### Story 3: Recover and retain context

**AC 3.1: Access and no mutation**
> Given another owner's plan, a private snapshot without access or a game login,
> when an assistant request is made,
> then existing access rules refuse it without returning source facts; permitted
> requests cannot mutate plans, snapshots, agreements, reviews or external systems.

**AC 3.2: Stale response ownership**
> Given a pending answer,
> when the user changes scope, leaves the view, signs out or asks another question,
> then the old answer cannot replace the current context or navigate the user.

**AC 3.3: Failure recovery and accessible controls**
> Given a failed request,
> when the manager retries,
> then the question and context remain available; keyboard and 360px layouts
> support asking, reading sources and returning to the underlying workflow.

**AC 3.4: Untrusted text**
> Given hostile instructions or markup in questions or source records,
> when evidence is rendered or processed,
> then it remains data, cannot trigger tools or writes, and cannot introduce
> arbitrary executable markup or model-authored navigation destinations.

**AC 3.5: Saved-input disclosure**
> Given unsaved editing controls or an upload preview,
> when the assistant opens,
> then it states that answers use saved inputs; upload previews must be saved or
> discarded before querying, and a changed saved revision invalidates an answer.

**AC 3.6: Comparison and scope edge cases**
> Given unplaced work on either side, absent or changed period origins, or an
> unknown team/initiative selector,
> when an answer is requested,
> then unavailable date movement is withheld, calendar origins are disclosed,
> and unknown selectors fail instead of producing a reassuring empty answer.

## 5. Functional Requirements

| ID | Requirement | Priority |
|---|---|---|
| FR-001 | Answers MUST show plan, calculation context and supporting source links. | MUST |
| FR-002 | Scheduling explanations MUST use existing scheduling policy and constraints. | MUST |
| FR-003 | Review answers MUST retain explicit snapshot/manual, date, timezone and scope. | MUST |
| FR-004 | The assistant MUST preserve existing authorization and remain read-only. | MUST |
| FR-005 | Missing evidence MUST remain distinguishable from measured or modeled success. | MUST |
| FR-006 | Users MUST be able to recover from failed requests without losing their question. | MUST |
| FR-007 | Source links MUST lead to existing authorized workflows with explicit context. | MUST |
| FR-008 | User-visible launches MUST include contextual documentation and feature discovery. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|---|---|---|---|
| NFR-001 | Accessible responsive interface | Keyboard operation and 360px light/dark layout | Playwright |
| NFR-002 | Reliable evidence boundaries | No unauthorized facts, invented dates or writes in acceptance cases | Ginkgo/Gomega |
| NFR-003 | Bounded requests | Explicit size, concurrency and duration limits for external processing if enabled | Integration tests |

## 7. Data Model

**Assistant request:** task is `schedule`, `changes`, `review` or `question`;
question is bounded to 2000 UTF-8 bytes, initiative/team are exact saved names.
Review requests require reviewDate, timezone and snapshotId, with `manual` the
explicit no-snapshot choice. `expectedFingerprint` identifies the saved inputs
shown when the panel loaded. Optional external question interpretation requires
`allowExternal` on that request.

**Assistant answer:** context identifiers, evidence fingerprint, supported task,
summary, categorized evidence, known gaps and server-constructed source links.
Answers are ephemeral; canonical facts and completed reviews remain in their
existing stores.

## 8. API Contract

| Method | Path | Description | Request | Response |
|---|---|---|---|---|
| GET | /api/plan/{id}/assistant | Read saved context and availability | — | Plan fingerprint and authorized scope choices |
| POST | /api/plan/{id}/assistant | Prepare a read-only grounded answer | Assistant request | Assistant answer |

## 9. Out of Scope

- Applying plan changes, completing reviews, sending messages or writing to Jira.
- Autonomous planning, invented estimates, cross-plan access or new forecast math.
- Persisted chat transcripts and an additional action/task catalog.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|---|---|---|---|
| Q1 | External AI for natural-language questions or guided local explanations? | Maintainer | 2026-09-07 | Resolved for this increment by Decision 4: guided local questions plus optional deployment-configured interpretation. Live enablement and provider validation remain deployment choices. |

## 11. Decision Record

### Decision 1: Reuse authoritative read models

**Context:** The assistant must explain the product without becoming a second
scheduler or review engine.

**Decision:** Assemble evidence through the existing saved-plan scheduling,
agreement comparison and weekly-review preparation functions. Build source
links from authorized identifiers. Keep the response structured and render text
with escaping. No assistant operation applies a change.

**Alternatives considered:** Recalculate results in prose or accept model-authored
links — rejected because they would bypass calculation and navigation authority.

**Consequences:** The first increment has a bounded set of useful workflows and
can be tested against exact domain results. Model integration, if selected,
cannot grant broader capabilities than those workflows.

### Decision 2: Contextual Bootstrap presentation

**Context:** Managers already work in Plan commitments, timelines and execution
review. Another global dashboard would separate answers from their evidence.

**Decision:** Add a Bootstrap assistant panel inside the saved-plan workflow,
with contextual entry points, scope selectors, guided questions, source actions
and accessible request status. Dispose pending answers on view/context changes.

**Alternatives considered:** A global floating chatbot — deferred because its
implicit context makes wrong-plan and wrong-snapshot answers harder to notice.

**Consequences:** Existing theme, navigation, permissions and documentation remain
shared. New factual capabilities require explicit domain evidence support.

### Decision 3: Conservative comparison and scoped review facts

**Context:** Legacy comparison deltas use integer weeks even for missing placement
and different calendar origins. Review evidence contains full-plan observations
although its agenda is scoped.

**Decision:** Use the shared comparator for matching, ranks and verdicts, but
withhold date deltas when either placement is missing or origins differ/are
absent. Show both contexts and a clear limitation. Read review facts from scoped
agenda/actions, labeling plan-wide items; omit its unfiltered raw evidence.
Validate requested selectors against the saved plan. Recheck plan fingerprint
and access after any optional external request before assembling facts.

**Alternatives considered:** Narrating the legacy comparison or complete review
payload without qualification — rejected because it could report an improvement
from unplaced work or imply unrelated evidence belongs to a selected team.

**Consequences:** Some comparisons are explicitly unknown. The assistant reveals
existing model limits rather than introducing a competing date calculation.

### Decision 4: Optional bounded question interpretation

**Context:** No application model provider is currently configured. Guided
questions must work without external setup, while natural-language questions
need an explicit deployment choice and per-request disclosure.

**Decision:** An optional OpenAI Responses adapter routes the question to one of
the three supported tasks and exact authorized initiative/team names. It receives
only the question and these names, not schedules, captures or credentials from
other integrations. The user consents before sending. Model text never becomes
an answer, link, calculation or write. Unknown tasks, names, malformed/refused
outputs and changed access/context fail clearly; guided questions remain usable.
Require deployment-provided model and API key rather than pinning an unverified
model choice. Set store:false, use a 20-second deadline, four concurrent model
requests and bounded request/response sizes, and refuse redirects.
Reject blank or oversized interpretation inputs with HTTP 400 before contacting
the provider. Reserve HTTP 503 for provider availability, busy processing or
unusable provider output so callers can distinguish input correction from retry.

**Alternatives considered:** Full plan prose generation — deferred because
source links alone cannot prove generated arithmetic or causal claims. Silent
keyword matching — rejected because it would suggest broader understanding than
supported. A required external service — rejected because guided explanations
remain useful without it.

**Consequences:** Free-text understanding is bounded to supported tasks and
explicit scope. Live model quality is a separate deployment validation check.
API contract checked 2026-09-07 against
https://developers.openai.com/api/docs/guides/structured-outputs.

### Decision 5: Retain review and request ownership

**Context:** Loading scope choices, interpreting a question and displaying an
answer have different lifetimes. A review link is useful only if it preserves
the evidence selections that produced the agenda. Missing catalog entries must
not erase a requested snapshot, and late plan operations must preserve the
current visible workflow.

**Decision:** Scope edits invalidate answers without canceling context catalogs.
Leaving the plan view invalidates pending answers. Review source links retain
snapshot/manual choice, date, timezone, team and initiative, including empty
agendas. Answer identity includes the canonical review evidence fingerprint.
Keep an explicitly requested snapshot selected as unavailable when absent from
the catalog; never replace it with another capture or manual review. Honor an
explicitly cleared selection on refresh. Route completed plan operations through
the current view and guard asynchronous renderers before painting.
Read current persisted roles before exposing context and again after external
interpretation; a team participates only when its work cell is in the path.

**Alternatives considered:** One request counter for every operation and token
roles alone — rejected because scope edits could strand loading controls and
revoked permissions could remain effective during external interpretation.

**Consequences:** Guided recovery stays usable while evidence and access changes
cannot silently inherit an earlier answer's authority.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|---|---|---|---|
| Supported answers with source context | Manual cross-view inspection | Every supported answer | Acceptance cases |
| Assistant-triggered writes | No assistant | Zero | Database before/after assertions |
| Recovery after context changes | No assistant | No stale answer display | Browser races |
