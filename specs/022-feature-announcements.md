# Feature announcements and discovery

**Status:** Implemented
**Author(s):** Project maintainers
**Date:** 2026-09-06
**Story/Ticket:** Feature discovery after login
**Sprint/Cycle:** Planning usability

---

## 1. Overview

Show signed-in users a brief introduction to newly available features and guide
them to those features with a yellow menu indicator. Remember separately whether
each user has received an announcement and whether they have visited its feature.

---

## 2. Problem

A manager can return after an update without noticing that linked planning
sources are available. Repeating the same popup on every login is distracting;
removing all guidance when the popup closes makes the feature difficult to find.

---

## 3. User Stories

### Story 1: Learn about relevant changes

**As a** signed-in user
**I want** a short introduction to newly available features
**So that** I can discover improvements relevant to my permissions.

### Story 2: Find and revisit a feature

**As a** returning user
**I want** persistent menu guidance and a What's new entry
**So that** I can explore a feature later without repeated interruptions.

---

## 4. Acceptance Criteria

### Story 1: Learn about relevant changes

**AC 1.1: First introduction**
> Given an authenticated user with an eligible unannounced feature,
> when the application finishes login and loads the catalog,
> then one accessible popup briefly describes the new features and their destinations.

**AC 1.2: Persistent acknowledgement**
> Given a successfully acknowledged announcement,
> when that user reloads or logs in again,
> then it does not automatically reopen; another user retains their own unseen state.

**AC 1.3: Eligibility and failure**
> Given a feature outside a user's current permissions or unavailable configuration,
> when the catalog loads,
> then it is omitted; a failed catalog or acknowledgement request does not block normal navigation.

### Story 2: Find and revisit a feature

**AC 2.1: Announcement and visit are distinct**
> Given an announced feature that the user has not visited,
> when its popup is closed,
> then its yellow menu indicator remains until the feature destination is opened.

**AC 2.2: Menu and replay**
> Given eligible features, whether previously announced or visited,
> when the user opens What's new,
> then they can replay the descriptions and open destinations without resetting saved state.

**AC 2.3: Keyboard and assistive technology**
> Given a keyboard or screen-reader user,
> when an announcement opens,
> then its title is announced, focus stays inside it, Escape closes it, focus returns to the trigger,
> and menu indicators have text equivalents rather than relying on yellow alone.

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | The system MUST present eligible unannounced features once per authenticated user after successful acknowledgement. | MUST |
| FR-002 | Announced and visited state MUST persist independently across reloads and sessions. | MUST |
| FR-003 | A user MUST NOT read or change another user's feature state. | MUST |
| FR-004 | Announcing a feature MUST NOT remove its unvisited menu guidance. | MUST |
| FR-005 | Menu guidance MUST indicate an unvisited eligible feature and clear after a successful visit. | MUST |
| FR-006 | Users MUST be able to replay eligible announcements through What's new. | MUST |
| FR-007 | Feature discovery MUST respect current role permissions and actual feature availability. | MUST |
| FR-008 | The popup and menu guidance MUST be accessible by keyboard and assistive technology. | MUST |
| FR-009 | Discovery failures MUST leave the rest of the application usable and MUST NOT claim that unpersisted acknowledgements were saved. | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Idempotent acknowledgement | Repeated identical acknowledgements preserve the original state | API regression test |
| NFR-002 | User isolation | No cross-user state access or mutation | Two-user API test |
| NFR-003 | Accessible presentation | No keyboard trap outside the modal; visible focus and named controls | DOM and browser tests |
| NFR-004 | Safe content | Catalog text cannot execute markup or navigate to external destinations | Rendering and route validation tests |

---

## 7. Data Model

**FeatureAnnouncement**
- `id`: stable string identifying one announcement; a new announcement uses a new ID.
- `title`, `description`: short plain text.
- `action`: `{type: "route" | "menu", route?, target?, parent?}`.
- Eligibility: server-owned role and availability rules; never supplied by the client.

**FeatureState**
- Authenticated subject plus feature ID form the unique identity.
- `announcedAt`, `visitedAt`: independently nullable server timestamps.
- API projections expose `announced` and `visited` booleans.

---

## 8. API Contract

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| GET | `/api/announcements` | Eligible catalog and caller state | None | `{features: [{id,title,description,action,announced,visited}]}` |
| POST | `/api/announcements/ack` | Record caller acknowledgement | `{id,kind: "announced" | "visited"}` | Updated state `{id,announced,visited}` |

Requests without authentication receive 401. Unknown or currently ineligible
IDs receive 404; malformed bodies or acknowledgement kinds receive 400. The
server derives the subject from authentication; request fields cannot select a
different subject. Repeated acknowledgements succeed without changing their
first recorded timestamp. A visited acknowledgement does not implicitly mark
the independent announced field.

---

## 9. Out of Scope

- Marketing campaigns, email, external messaging, or usage profiling.
- Announcements for capabilities not available to the current user.
- User-authored HTML, external links, or remote announcement catalogs.
- An interruption on every navigation or login.

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | When is a popup considered announced? | Maintainers | 2026-09-06 | Decision 2 records it after successful presentation and acknowledgement persistence. |
| Q2 | Should visits erase announcement history? | Maintainers | 2026-09-06 | No; Decision 1 keeps independent monotonic state. |

---

## 11. Decision Record

### Decision 1: Store independent state on the server

**Context:** Local browser flags cannot provide reliable per-user behavior across sessions or devices.

**Decision:** Persist first-announced and first-visited timestamps keyed by authenticated subject and stable feature ID. Both are monotonic and idempotent. Catalog reads apply the caller's current roles and deployment availability.

**Alternatives considered:**
- Browser-only storage — rejected because different browsers and shared devices produce inconsistent state.
- One seen flag — rejected because receiving an introduction is different from visiting a feature.

**Consequences:** Reloads and separate users are predictable. Removing permissions hides the feature without erasing historical state.

The persistence key is the stable authenticated subject, without a cascading
foreign key to the replaceable account table. Ordinary account saves currently
replace account rows; that maintenance must not erase feature acknowledgements.

### Decision 2: Keep automatic discovery brief and replayable

**Context:** Users want a brief post-login introduction and a way to return later.

**Decision:** Show a single modal containing eligible unannounced features. Acknowledge announced items after the modal is presented; persistence failure remains retryable. Closing the modal does not acknowledge visits. What's new replays eligible descriptions without resetting either state. Mark a visit when its actual allowed destination opens, not merely when its description is read.

**Alternatives considered:**
- A modal per feature — rejected because it creates repeated interruptions.
- Clearing dots on dismissal — rejected because the user may plan to explore later.

**Consequences:** A failed acknowledgement may cause another introduction after reload; the UI must not promise otherwise. Already-open feature destinations may acknowledge visits without opening another modal.

### Decision 3: Use a safe catalog and accessible menu guidance

**Context:** Feature descriptions and destinations must be trustworthy and usable.

**Decision:** Keep a static server-owned catalog with plain text, role filtering and known local route/menu actions. Display yellow dots with an accessible "New feature" label at the destination and its containing menu. Reuse the existing modal focus management and render text through escaping or textContent.

**Alternatives considered:**
- Remote HTML and arbitrary URLs — rejected because discovery does not require them.

**Consequences:** New releases add stable catalog entries and explicitly identify their existing application destinations. No user must infer meaning solely from color.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Repeated acknowledged introductions | No persistent discovery contract | Zero after successful acknowledgement | Reload and login regression |
| Cross-user state leakage | Not covered | Zero | API isolation tests |
| Keyboard access to discovery | Not covered | Complete popup and replay path | Browser walkthrough |

---

## Review Checklist

- [x] Problem and user stories are explicit.
- [x] Acceptance criteria cover persistence, failure and accessibility.
- [x] Requirements describe outcomes; decisions precede implementation.
- [x] Authentication, state isolation and replay semantics are defined.
- [x] Tests and implementation have separate authors.
