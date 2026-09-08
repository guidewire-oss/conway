# Consistent plan controls and roster-aware samples

**Status:** Done
**Author(s):** Anoop Gopalakrishnan, Codex
**Date:** 2026-09-07
**Story/Ticket:** Maintainer request: plan control consistency and sample downloads
**Sprint/Cycle:** Planning usability

## 1. Overview

Plan commitments and Timeline should present controls with consistent sizing
and alignment. An initiatives sample downloaded inside a plan should use that
plan's attached teams, reducing the work needed to prepare an import.

## 2. Problem

Legacy style overrides shrink Assumptions and timeline searches independently
of surrounding buttons. Generic sample team names also require managers to
rebuild team columns after selecting their own roster.

## 3. User Stories

### Story 1: Consistent controls

**As a** manager, **I want** aligned, equally sized peer controls **so that**
their appearance communicates their role without accidental visual emphasis.

### Story 2: A sample matching my teams

**As a** planner, **I want** the initiatives sample to match my attached roster
**so that** I can replace example initiatives and import it directly.

## 4. Acceptance Criteria

**AC 1.1:** Given Plan commitments, when its actions render, then Assumptions,
Preview optimized order and Open timeline have equal font sizes and heights.

**AC 1.2:** Given Timeline in either grouping, when controls fit on one line,
then both search inputs and adjacent buttons have equal heights and aligned
bottom edges; narrow screens wrap without clipping inputs or their labels.

**AC 2.1:** Given a plan with attached teams, when its initiatives sample is
downloaded, then the workbook contains exactly those teams' column pairs and
generic example work that imports without unknown teams.

**AC 2.2:** Given no attached teams, when the sample is downloaded, then the
existing demo sample is returned. After switching rosters successfully, the
next sample uses the replacement teams. A pending or failed roster change
must not misleadingly download a template for the requested roster.

**AC 2.3:** Given a private plan, when a caller without plan access requests
its sample, then the request is refused. A failed download shows an actionable
message and can be retried without changing plan inputs.

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | Peer controls MUST use consistent sizing and alignment. | MUST |
| FR-002 | The plan sample MUST use its saved team snapshot, including uploaded and linked teams. | MUST |
| FR-003 | A plan with no teams MUST receive the existing demo sample. | MUST |
| FR-004 | Sample downloads MUST enforce existing plan permissions and MUST NOT include real initiatives. | MUST |
| FR-005 | Samples MUST preserve the supported workbook import format and explain that example work must be replaced. | MUST |
| FR-006 | Downloads MUST report failures and remain retryable. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Consistent peer geometry | Within 1 CSS pixel at desktop sizes | Playwright bounding boxes, both themes |
| NFR-002 | Responsive controls | No toolbar overflow at 390 CSS pixels | Playwright |
| NFR-003 | Workbook compatibility | All sample work resolves to supplied teams | Ginkgo/Gomega round trip |

## 7. Data Model

No persistence changes. The existing plan team snapshot supplies names and
order. Downloading a sample does not modify the plan or its reusable roster.

## 8. API Contract

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| GET | /api/plan/{id}/sample/initiatives.xlsx | Authorized plan sample | Existing manager authentication | XLSX attachment; existing 401/403/404 access responses |
| GET | /api/sample/initiatives.xlsx | Unchanged public demo sample | None | XLSX attachment |

## 9. Out of Scope

- Live reusable-roster synchronization, provider OAuth, scheduling changes.
- Exporting actual initiative data or creating separate worksheets per team.
- Redesigning unrelated screens or changing Bootstrap versions.

## 10. Open Questions

None. The existing matrix format resolves the request for roster-based sheets
as paired team columns in a single importable worksheet.

## 11. Decision Record

### Decision 1: Bootstrap owns peer control geometry

**Context:** ID-specific font sizing and custom input padding override Bootstrap.
**Decision:** Remove those overrides and use Bootstrap layout utilities to align
the timeline toolbar, preserving visible field labels and wrapping.
**Alternatives considered:** Hardcoded equal heights would duplicate framework
sizing and break with theme or font changes.
**Consequences:** Controls follow the installed framework's default size.

### Decision 2: Derive samples from the authorized plan snapshot

**Context:** Selecting a roster immediately attaches a frozen team copy to the
plan; later reusable-roster edits need not be part of this plan.
**Decision:** Add a GET subroute behind existing plan authorization. Use the
saved team snapshot and generic example work, with demo fallback only when
there are no attached teams. Disable sample download during roster attachment.
Roster attachment must apply the existing owner/admin/public read boundary
before copying team names; plan ownership alone does not grant roster access.
Pending roster/sample operations survive view rendering. An uncertain roster
response requires reloading saved plan inputs before another sample is offered;
the interface must not infer that a failed response means the write failed.
Confirmed errors retain the server's actionable message. Empty rosters receive
a validation error. Sample discovery is acknowledged after a successful
download, not merely by opening Plan setup.
**Alternatives considered:** A public roster-ID query risks exposing private
team names; reading the live roster could disagree with the plan's inputs.
**Consequences:** Uploaded and linked rosters work identically; samples contain
no real initiatives and preserve the single-sheet FullKit matrix format.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Peer control size discrepancies | Two reported groups | Zero | Browser geometry assertions |
| Manual team-column replacement | Required for own roster | None | Download and import journey |

## Review Checklist

- [x] Problem and user value stated
- [x] Acceptance criteria cover success, failure and access boundaries
- [x] Requirements specify WHAT and WHY
- [x] Non-functional thresholds are measurable
- [x] Scope and decisions are recorded
- [x] Behavioral and browser checks pass
- [x] User documentation and announcement updated
