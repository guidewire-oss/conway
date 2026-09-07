# Reliable evidence foundation

**Status:** Implemented; acceptance passed; pull request pending
**Author(s):** Anoop Gopalakrishnan, Codex
**Date:** 2026-09-07
**Story/Ticket:** Product roadmap: reliable evidence foundation
**Sprint/Cycle:** Not assigned

## 1. Overview

Managers can save a Jira capture source, schedule dated evidence, recover failed
captures, and inspect stable source-scoped team and issue identities. Successful
captures enter the existing snapshot picker without changing a selected plan,
execution snapshot, agreement or completed review.

## 2. Problem

Manual imports keep their jobs in memory and discard credentials. A restart can
lose progress, and repeated captures do not identify a durable source. Managers
need to distinguish old evidence from a failed refresh without mistaking either
for zero delivery. Team renames must not silently join unrelated teams.

## 3. User Stories

### Story 1: Keep evidence current
**As a** manager **I want** a saved capture schedule **so that** reviews have dated evidence.
### Story 2: Recover safely
**As a** source owner **I want** durable attempt history and retry **so that** failed captures never replace usable evidence.
### Story 3: Preserve identity
**As a** team lead **I want** explicit team aliases and stable issue IDs **so that** a renamed team or issue key does not erase its identity.

## 4. Acceptance Criteria

**AC 1.1: Scheduled capture**
> Given a saved source and authorized owner, when its interval is due, then one
> attempt captures its projects and pinned roster and publishes a complete private snapshot.

**AC 1.2: Explicit selection**
> Given an existing selected snapshot, when a source completes, then the new
> snapshot is available but existing selections and agreements remain unchanged.

**AC 1.3: Freshness**
> Given a last success and freshness threshold, when its age reaches the
> threshold, then the source is stale even if its most recent attempt succeeded.
> No success means no evidence; capture state is separate from freshness.

**AC 2.1: Failure and retry**
> Given a previous success, when a later capture fails, then the previous success
> remains selectable, a safe error and attempt time persist, and the owner can retry.

**AC 2.2: Restart and concurrency**
> Given a claimed attempt, when a worker disappears or another worker competes,
> then a lease prevents duplicate publication and an expired attempt is recorded
> as interrupted before a replacement starts. A late worker cannot publish.

**AC 2.3: Access and credentials**
> Given another user's private source or an expired/non-manager owner, when a
> caller reads, edits or runs it, then access is denied; credentials never appear
> in responses, stored run errors or logs. Saved tokens require explicit consent.

**AC 3.1: Stable identities**
> Given two captures of the same source, when a Jira issue key changes but its
> provider ID remains, then its identity remains. Team aliases explicitly assigned
> to one team preserve its ID; ambiguous aliases are rejected. Unmatched names
> remain visible as unmapped instead of being silently merged.

**AC 3.2: Immutable lineage**
> Given a completed capture, when its source configuration changes, then its
> scope, roster and identity observations remain as captured.

## 5. Functional Requirements

| ID | Requirement | Priority |
|---|---|---|
| FR-001 | Sources MUST retain owner, site, projects, roster, counting policy, cadence and freshness threshold. | MUST |
| FR-002 | Source owners and admins MUST be able to pause, resume, capture now, replace credentials and inspect attempts. | MUST |
| FR-003 | Successful publication MUST atomically include data, identity observations and successful attempt state. | MUST |
| FR-004 | Failed attempts MUST retain the last complete evidence and offer recovery without publishing partial data. | MUST |
| FR-005 | Existing manual imports and explicit snapshot selection MUST remain available. | MUST |
| FR-006 | Identity MUST be scoped to the source; aliases MUST be explicit and unique within that source. | MUST |
| FR-007 | Source configuration changes MUST NOT rewrite prior captures. | MUST |
| FR-008 | Saved credential consent MUST explain persistence and scheduled read access. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|---|---|---|---|
| NFR-001 | Bounded work | 15-minute workload; 20-minute lease; no overlap per source | Ginkgo concurrency/recovery tests |
| NFR-002 | Cadence | Manual, 6 hours, 24 hours or 7 days; due work checked every minute | Clock-driven tests |
| NFR-003 | Credential protection | Authenticated encryption; no plaintext read API; HTTPS Jira Cloud only; no redirects | Unit/API tests |
| NFR-004 | Accessible workflow | Bootstrap controls, named inputs, keyboard access and 360px layout | Playwright |

## 7. Data Model

**EvidenceSource:** ID, owner, name, site, projects, roster ID, frozen roster
composition, WIP mode, interval hours, freshness hours, enabled, version,
explicit team identity/alias records, encrypted credential, next due timestamp.
**CaptureRun:** ID, source ID, status, started/finished times, lease deadline,
snapshot ID, safe error and captured configuration version.
**IdentityObservation:** source-scoped ID, kind (team or issue), provider ID or
explicit aliases, captured label/key; stored with the snapshot.
Sources have many attempts and completed snapshots; a failed attempt has no snapshot.

## 8. API Contract

| Method | Path | Description | Request | Response |
|---|---|---|---|---|
| GET/POST | /api/evidence-sources | List/create owned sources (admin sees all) | Source configuration on POST | Source metadata |
| GET/PUT | /api/evidence-sources/{id} | Read/update with version guard | Full editable configuration, version | Source with latest attempts |
| POST | /api/evidence-sources/{id}/capture | Claim a manual attempt | Empty | 202 attempt or 409 busy |
| GET | /api/snapshots/{id}/data/identities.json | Read captured identity observations | Existing snapshot authorization | Captured lineage |

## 9. Out of Scope

Cross-source or cross-plan canonical capacity reservations, transition/changelog
history, automatically switching execution evidence, Jira writes, notifications
to others, OAuth session persistence and automatic team-name similarity matching.
Scheduled sources use explicitly saved API-token credentials; the existing
interactive OAuth/manual import workflow remains separate and available.

## 10. Open Questions

None blocking the first increment. Live organization credentials remain a
separate deployment validation; isolated provider fixtures exercise release tests.

## 11. Decision Record

### Decision 1: Durable leases and atomic publication
Use PostgreSQL source-row locks to claim one attempt with a 20-minute lease.
The worker has a 15-minute context. Expired claims become interrupted; completion
checks the run token and lease under the same lock before publishing data and
status in one transaction. Next due is completion plus the configured interval;
failed scheduled attempts wait the same interval (manual retry is available).
Unrelated settings edits retain next due; cadence/enable changes reset it from
the save time. Expired claims may recover before that next due time.
A process restart resumes due work rather than inventing successful progress. A durable insertion sequence orders attempts newest first when timestamps tie.

### Decision 2: Explicit credentials and immutable scope
Store API tokens with AES-GCM using a domain-separated key derived from the
existing durable server secret, binding ciphertext to source ID. Never return
provider response text in run errors. Permit only HTTPS *.atlassian.net origins
without userinfo, ports, paths or redirects. Site is immutable; a new site needs
a new source. Pin a roster composition at source creation or explicit reselection.
Configuration updates require optimistic version matching and reject active runs.
Using in-memory OAuth sessions was rejected because a restart loses authorization.

### Decision 3: Identity without guessed associations
Each source creates stable team IDs from its pinned roster; owners can edit team
display names and aliases while keeping IDs. Each issue identity combines source
ID and Jira's provider ID; missing IDs fail scheduled publication. Jira keys and
summaries remain captured labels. Save the mapping in identities.json in the same
transaction as evidence; existing planning bindings continue using explicit keys.
Jira search ID/key and pagination contract checked 2026-09-07 against
https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/.
A completed run freezes the configuration rather than retroactively updating history.
Missing or ambiguous issue IDs fail a capture; incomplete or cycling pagination
fails rather than publishing truncated evidence.

### Decision 4: One existing entry point
Extend Measure > Snapshots with capture sources above dated captures. Show last
success, next capture, freshness and last attempt separately. Link successful
captures into the existing explicit snapshot selector; do not add competing
selection state. Read-only captured identity details use the same snapshot ACL.
Roster selection requires owner/public/admin read access. Captured source snapshots
cannot be deleted or have their roster reassociated; name and visibility remain
manageable through the existing snapshot controls. Current persisted owner roles
and account expiry are checked before work and again under a shared account lock
before publication; revoked access fails the attempt without publishing evidence.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|---|---|---|---|
| Lost attempt state after restart | Manual jobs are in memory | None for saved sources | Restart/lease acceptance |
| Partial capture publication | Atomic manual snapshots | Remains zero | Failed publication test |
| Unexplained stale evidence | Date only | Source freshness and recovery visible | Browser journey |
