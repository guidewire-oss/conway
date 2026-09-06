# Linked Google Sheets for planning inputs

**Status:** Implemented
**Author(s):** Project maintainers
**Date:** 2026-09-06
**Story/Ticket:** Captured planning sources and controlled updates
**Sprint/Cycle:** Planning usability

---

## 1. Overview

Link a Google Sheet range to a plan's team roster or initiatives. Capture changed
content as immutable Conway versions, validate it, and let managers review or
explicitly opt into applying safe updates without overwriting local work or agreements.

---

## 2. Problem

Repeated file uploads obscure which spreadsheet produced a plan and whether
new edits have arrived. Automatically replacing a working plan can lose local
estimates, pins or teams, while a misleading revision label can imply that
Conway has captured every change made in Google Sheets.

---

## 3. User Stories

### Story 1: Link and inspect a planning source

**As a** manager
**I want** to connect an accessible sheet and inspect dated captures
**So that** I can trace which source content produced my planning inputs.

### Story 2: Apply updates safely

**As a** manager
**I want** validated changes and local conflicts explained before replacement
**So that** I can refresh inputs without losing work or changing an agreement.

### Story 3: Control automation and restore earlier inputs

**As a** manager
**I want** optional safe automatic updates, pause/disconnect controls and captured history
**So that** I can control the source relationship and recover an earlier version.

---

## 4. Acceptance Criteria

### Story 1: Link and inspect a planning source

**AC 1.1: Connect without applying**
> Given a manager and a sheet shared with the configured server identity,
> when the manager links its range in review mode,
> then Conway captures and validates it without changing the working plan.

**AC 1.2: Dated, immutable captures**
> Given an existing source,
> when a check observes changed content,
> then a new captured version records its time, content hash and validation result;
> an unchanged check does not duplicate that version.

**AC 1.3: Invalid source evidence**
> Given malformed, duplicate or incomplete sheet data,
> when a check captures it,
> then errors identify why it cannot apply, the capture remains inspectable,
> and no existing planning inputs are discarded.

### Story 2: Apply updates safely

**AC 2.1: Explicit reviewed apply**
> Given a capture that validates against the current plan and its unchanged expected fingerprint,
> when the manager applies it,
> then the intended input kind changes atomically and its provenance is recorded.

**AC 2.2: Conflict after review**
> Given local inputs changed after the source's checkpoint or the explicit preview's fingerprint was captured,
> when an automatic or stale explicit apply is attempted,
> then the plan remains unchanged and the conflict requires a fresh review.

**AC 2.3: Scope reduction and agreements**
> Given an update removes existing initiatives, teams or assigned work,
> when it is checked,
> then it requires explicit review and removal acknowledgement, never automatic apply;
> every existing baseline remains byte-for-byte unchanged.

### Story 3: Control automation and restore earlier inputs

**AC 3.1: Opt-in auto-apply**
> Given auto-apply was explicitly enabled and the current plan matches the checkpoint,
> when a valid non-removing version is captured,
> then it may apply; invalid, empty, removal or conflicting updates remain pending.

**AC 3.2: Pause and disconnect**
> Given a paused or disconnected source,
> when polling is due,
> then no remote check or automatic update runs; recorded history remains available.

**AC 3.3: Restore a capture**
> Given an earlier capture that validates against current planning inputs and a fresh expected fingerprint,
> when the manager explicitly restores it with any necessary removal acknowledgement,
> then the working inputs are updated through the same validation and conflict checks,
> the restore is recorded, and neither prior captures nor agreements are rewritten.

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | Managers MUST be able to link a Google Sheet range as teams or initiatives for an authorized plan. | MUST |
| FR-002 | Source checks MUST capture changed content immutably with provenance and validation evidence. | MUST |
| FR-003 | Raw captures MUST remain distinguishable from valid, applyable input versions. | MUST |
| FR-004 | Source parsing MUST preserve supported planning fields and MUST report unrecognized or invalid structure instead of silently dropping meaningful data. | MUST |
| FR-005 | Review mode MUST be the default and MUST NOT change inputs merely by linking or checking. | MUST |
| FR-006 | Automatic apply MUST require explicit opt-in, valid nonempty inputs, unchanged local state and no removal of existing scope. | MUST |
| FR-007 | Explicit apply and restore MUST reject stale expected state and require acknowledgement of scope removal. | MUST |
| FR-008 | Applying a source MUST be atomic and MUST NOT mutate existing baselines. | MUST |
| FR-009 | Managers MUST be able to pause, resume and disconnect polling while retaining capture history. | MUST |
| FR-010 | Every source and capture MUST inherit plan access control; users MUST NOT access another plan's source through guessed identifiers. | MUST |
| FR-011 | The UI MUST show source status, mode, last check, pending validation/conflicts and last applied version. | MUST |
| FR-012 | The product MUST describe its history as Conway captures, not complete Google revision history. | MUST |
| FR-013 | Provider credentials MUST remain server-side and provider access MUST be read-only. | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Polling cadence | Default 15 minutes; configurable 5–1440 minutes | Clock-controlled poll test |
| NFR-002 | Isolation and concurrency | No cross-plan reads or writes; stale apply returns conflict | API and concurrent apply tests |
| NFR-003 | Idempotent capture | No duplicate version for unchanged latest content | Repeated check test |
| NFR-004 | Atomicity and history | Failed apply changes no plan fields; earlier versions remain unchanged | Stored-state assertions |
| NFR-005 | Deterministic verification | Provider/network/time replaceable through internal seams, no public fixture endpoint | Go provider and browser test harness |
| NFR-006 | Credential protection | No credentials in API payloads, browser storage, logs or committed fixtures | Schema and content assertions |

---

## 7. Data Model

**PlanSource**
- `id`, `planId`, `kind`: `teams` or `initiatives`.
- `provider`: `google_sheets`; `spreadsheetUrl`, normalized spreadsheet ID and `range`.
- `mode`: `review` or `auto_apply`; `status`: `active`, `paused` or `disconnected`.
- `pollMinutes`, last/next check time, last error, and latest captured/applied version IDs.
- A checkpoint fingerprint identifies the plan state that automatic updates may replace.
- At most one active or paused source for each plan and input kind.

**SourceVersion**
- `id`, `sourceId`, capture time, content hash and raw cell content.
- Parsed input representation, `valid`, `errors`, `warnings`, and change/removal summary.
- Capture content and validation evidence are immutable; applying references a version rather than rewriting it.

**SourceApplication**
- Source/version IDs, actor or automated origin, time, previous/resulting fingerprints and restore/removal acknowledgement.
- Existing baseline snapshots are independent immutable records.

---

## 8. API Contract

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| GET | `/api/plan/{plan}/sources` | List authorized plan sources and provider readiness | None | `{sources,configured,serviceAccountEmail?}` |
| POST | `/api/plan/{plan}/sources` | Link and initially capture | `{kind,spreadsheetUrl,range,mode?,pollMinutes?}` | `{source,version?}` |
| PATCH | `/api/plan/{plan}/sources/{source}` | Change mode/cadence or pause/resume | `{mode?,pollMinutes?,status?: "active" | "paused"}` | `{source}` |
| POST | `/api/plan/{plan}/sources/{source}/check` | Check now | `{}` | `{source,version?,unchanged,applied?,conflict?}` |
| GET | `/api/plan/{plan}/sources/{source}/versions` | Captured history | None | `{versions}` |
| GET | `/api/plan/{plan}/sources/{source}/versions/{version}` | Inspect captured values and validation | None | `{version,planFingerprint}` |
| POST | `/api/plan/{plan}/sources/{source}/apply` | Apply or restore a valid capture | `{versionId,expectedFingerprint,allowRemovals?:boolean}` | `{source,versionId,planFingerprint}` |
| DELETE | `/api/plan/{plan}/sources/{source}` | Disconnect; retain history | None | `{source}` |

Authentication is required. Plan role/access checks apply to every nested ID.
Malformed configuration returns 400; inaccessible/mismatched IDs return 404 or
the application's existing authorization refusal; stale fingerprints and
unacknowledged removals return 409; invalid captured content cannot apply and
returns 422. Missing provider configuration returns a useful unavailable state
and rejects remote operations without changing inputs. Client data cannot supply
credentials, application actor, plan ownership or a trusted validation verdict.

---

## 9. Out of Scope

- Editing Google Sheets, personal OAuth accounts or domain-wide delegation.
- Complete native Google revision history or recovery of edits between polls.
- Mutating global roster records, snapshot evidence or saved agreements.
- Silent empty-plan replacement, implicit scope deletion or automatic conflict merging.
- A production endpoint for injecting fixture data or arbitrary provider hosts.

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | What should happen when a sheet removes work? | Maintainers | 2026-09-06 | Decision 3 requires review and explicit removal acknowledgement. |
| Q2 | What history can Conway claim? | Maintainers | 2026-09-06 | Decision 2 retains observed captures only, including invalid captures. |
| Q3 | What polling interval is appropriate? | Maintainers | 2026-09-06 | Decision 4 defaults to 15 minutes with a 5-minute minimum. |

---

## 11. Decision Record

### Decision 1: A server-configured, read-only provider

**Context:** Managers want to link private spreadsheets without pasting credentials into planning forms.

**Decision:** Use the server's configured Google service account and the `https://www.googleapis.com/auth/spreadsheets.readonly` scope. Users share the required spreadsheet with that identity. Normalize an allowlisted Google Sheets URL and read its specified range; never fetch arbitrary user-provided hosts. Report missing configuration and permission failures explicitly. No domain-wide delegation is required or requested.

**Alternatives considered:**
- Public CSV links — rejected as the sole method because private sheets should remain private.
- Personal OAuth and write access — outside this bounded request.

**Consequences:** Operations depend on server configuration and sharing permissions. Google's scope applies to a spreadsheet, not a single tab; the requested range restricts what Conway reads. Sources: [Google Sheets scopes](https://developers.google.com/workspace/sheets/api/scopes) and [service-account authorization](https://developers.google.com/identity/protocols/oauth2/service-account), checked 2026-09-06.

### Decision 2: Immutable observed captures and parser parity

**Context:** Polling cannot reconstruct every remote edit, and invalid input is still useful evidence.

**Decision:** Store a dated raw capture for changed cell content, deduplicating identical consecutive hashes. Reuse the existing roster and initiative import semantics, retaining supported fields and explicit validation diagnostics. Keep raw captures distinct from valid applyable inputs. A blank capture is invalid; duplicate identifiers and rows whose meaningful input would be silently discarded cannot apply. A return to older content after an intervening change is another observation, not proof of a native revision sequence.

**Alternatives considered:**
- Calling captures Google revisions — rejected because edits between checks are not observed.
- Keeping only successfully parsed data — rejected because it hides failed upstream changes.

**Consequences:** History can explain both successful and refused updates. Google errors are recorded as check errors, not fabricated empty captures.

Recognized unestimated markers such as `TBD` retain assigned work with a warning;
they are not blank cells. Match roster identifiers after trimming and case
normalization, but reject unresolved dependency names, unknown meaningful columns
and invalid effort values rather than dropping them.

### Decision 3: Compare-and-apply protects local work and agreements

**Context:** Local planning edits can occur while a sheet changes or a manager reviews a version.

**Decision:** Review is the default. Explicit apply/restore requires the current plan fingerprint and repeats validation transactionally. Automatic apply requires opt-in and exact agreement with the source checkpoint established at linking or its last successful application. Any local divergence blocks automatic replacement. Removed teams, initiatives or assigned work require explicit `allowRemovals`; empty captures never apply. Baselines are never modified. Apply only the source's input kind and retain unrelated plan fields.

**Alternatives considered:**
- Latest remote content always wins — rejected because it loses local edits.
- Automatic field-by-field merging — rejected because conflicting intent is not safely inferable.

**Consequences:** Review may be required after local changes. Restoring an old capture is a new application event and uses the same checks, preserving history and prior agreements.

Matching initiatives retain their current start/lane pins. An absent epic-key
column preserves current bindings. Revalidate the captured input kind against
the live counterpart at apply time; a captured roster never restores stale
initiatives. Removing a team still referenced by current assigned work is invalid.

### Decision 4: Controlled polling and deterministic provider seams

**Context:** Updates should arrive without manual uploads while remaining bounded and testable.

**Decision:** Poll active sources at 15 minutes by default, accepting 5–1440 minute intervals. Pause/disconnect stop polling; disconnect is soft and retains captures. Serialize checks/applications per source and use transactional compare-and-apply against the plan. Inject provider fetch and clock dependencies internally for deterministic tests; mocks do not become production HTTP features. A manual check may inspect a paused source but must not auto-apply until resumed.

**Alternatives considered:**
- Browser timers only — rejected because updates stop when the tab closes.
- Public test-import routes — rejected because the test seam must not bypass production authorization.

**Consequences:** The server owns polling, credentials and conflict protection. Tests can exercise remote failures, changing captures, concurrent local edits and historical restores without real credentials.

The internal provider seam is `Provider.Fetch(context.Context, spreadsheetID,
rangeName string) ([][]string, error)`. `ParseLink` validates and normalizes the
Google link; `Parse(kind, rows, current BaselineInputs)` returns a candidate with
teams, initiatives, errors, warnings, removals, count and a validity predicate.
These are internal Go seams; no test data can be injected through a public API.

### Decision 5: Browser acceptance uses an isolated real server

**Context:** Parser and handler tests alone cannot demonstrate linking, review,
conflict and restoration through the manager's controls.

**Decision:** A Go acceptance test starts an `httptest` server with the real
authenticated product handlers, static application and an internal mock Sheets
provider. Playwright drives this server against an isolated PostgreSQL database.
The test is opt-in through `CONWAY_TEST_BROWSER=1`; `PLAYWRIGHT_MODULE` selects
the installed test runner and optional `PLAYWRIGHT_BROWSER_CHANNEL` selects a
local browser. CI installs bundled Chromium and leaves the channel unset.
No test fixture routes or network override configuration enter production code.

**Alternatives considered:** Browser route mocks for all APIs were rejected as
the sole acceptance layer because they cannot exercise persistence or plan
conflict checks.

**Consequences:** CI runs database and JavaScript checks as well as the browser
acceptance test. Browser test dependencies live outside the product package.

### Decision 6: Fresh explicit review can recover a context-dependent invalid capture

**Context:** An initiative capture can be invalid because a required team is
absent from the current roster. Correcting that roster can make the same cells
usable without changing the historical evidence.

**Decision:** Explicit apply and restore use the candidate revalidated against
the current plan and require that preview's unchanged fingerprint. Historical
validity, errors, warnings and captured cells remain immutable and are displayed
separately from current validation. A currently valid candidate may be explicitly
applied even if invalid when captured. Intrinsic errors, empty captures, stale
fingerprints and unacknowledged removals still refuse application. Automatic
application additionally requires validity at capture time; it never silently
recovers a historically invalid version.

Requester tier cells, when populated, must be integers 1 through 4; a blank
cell represents the unset value. Application provenance must reference a
version belonging to the same source, enforced by a composite database foreign
key as well as scoped handler lookups.

**Alternatives considered:** Permanently rejecting any historically invalid
capture was rejected because it prevents an explicit reviewed recovery after
the missing roster context is corrected. Automatically recovering such captures
was rejected because the earlier validation failure merits human review.

**Consequences:** Managers can repair context and review the original capture;
historical errors remain visible, automatic updates remain conservative, and
cross-source provenance cannot be inserted even below the HTTP layer.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Repeated manual uploads | Required | Linked validated capture available within configured polling interval | Controlled provider test |
| Lost local work on refresh | No linked-source contract | Zero conflicts overwritten automatically | Concurrent state regression |
| Mutated agreements | Must remain zero | Zero | Baseline immutability assertion |
| Untraceable applied input | Manual filename only | Every linked application identifies a capture | History and API assertions |

**Validation boundary:** Automated acceptance uses an internal deterministic
provider with the real application, authenticated handlers and PostgreSQL.
Live Google service-account access has not been credential-tested; deployment
owners must confirm spreadsheet sharing and provider configuration separately.

---

## Review Checklist

- [x] Problem and user stories are explicit.
- [x] Happy paths, invalid captures, conflicts and restores have acceptance criteria.
- [x] Provider access, authentication and plan isolation are defined.
- [x] Capture history is distinguished from Google revision history.
- [x] Decisions precede implementation; tests and implementation have separate authors.
