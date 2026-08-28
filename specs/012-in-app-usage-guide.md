# In-App Usage Guide and Onboarding

**Status:** In Progress
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-26
**Story/Ticket:** user request; follows specs/009 IA pass
**Sprint/Cycle:** n/a

---

## 1. Overview

The planning workflow is now a full loop — setup, order, optimize/pin, drag,
baseline, compare, timeline — but nothing in the app teaches that loop. The
Guide modal routes by persona but froze before planning existed; vocabulary
tooltips cover words, not procedure. This spec adds a **Planning Manager
persona** to the Guide (clickable steps that navigate), a **dismissible
first-visit callout** on Order and Timeline, an **in-app docs panel** (the
manual, shipped offline in `app/`), and **deep links** from warnings into it.

---

## 2. Problem

A brand-new planning manager opening the Portfolio plan sees a finished, dense
tool with no sense of the intended sequence. The knowledge exists — in specs,
commit messages, and the team's heads — but not where a user can reach it.
Warnings name problems ("no period start", "missing pods") without teaching
the workflow that prevents them.

---

## 3. User Stories

### Story 1: Learn the loop in the app

**As a** first-time planning manager
**I want** a Guide persona that walks the planning loop as clickable steps
**So that** I learn the workflow by doing it, not by reading a wiki

### Story 2: Read the manual in the app

**As a** planner
**I want** an in-app usage guide covering the ritual, the models, the
verdicts and the interactions
**So that** answers are one click away without leaving the tool

### Story 3: Warnings link to their explanation

**As a** planner hitting a warning
**I want** a "learn more" link that opens the docs at that section
**So that** the fix comes with the why

---

## 4. Acceptance Criteria

**AC 1.1: The persona routes by click**

> Given the Planning Manager persona in the Guide
> When a step is clicked
> Then the app navigates to the step's view (and opens its dialog where the
> step says so)

**AC 2.1: The docs panel is offline-complete**

> Given no network
> When the docs panel opens
> Then every section renders (the content ships inside `app/`)

**AC 3.1: Warnings deep-link**

> Given the missing-pod or no-period-start warning
> When "learn more" is clicked
> Then the docs panel opens scrolled to the matching section

**AC 3.2: First-visit callouts dismiss for good**

> Given a first visit to Order or Timeline
> When the callout is dismissed
> Then it does not re-appear for the rest of the session (sessionStorage —
> a fresh session shows it again, which is intentional for new planners)

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The Guide modal MUST gain a Planning Manager persona whose steps navigate on click | MUST |
| FR-002 | A docs panel (theme-styled overlay, content shipped in `app/docs.html`) MUST open from the Guide and from setup-card "learn more" links | MUST |
| FR-003 | Warnings (missing pods, no period start, beyond-horizon) MUST carry "learn more" deep links into the panel | MUST (beyond-horizon gets its own link: it fires alongside no-dates only when the plan also lacks a period start, so the two banners are not interchangeable) |
| FR-004 | First-visit callouts on Order and Timeline MUST be dismissible and session-persistent | SHOULD |
| FR-005 | The panel MUST cover: the planning ritual, effort model & chunking, verdicts & fever chart, baselines & comparison, pins/drags/filters, WIP models & stagger | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression | suites green | `node --test`, `go test ./...` |
| NFR-002 | Panel opens instantly, no network | offline render | in-browser |

---

## 7. Data Model

- Session-persistent dismiss flags: `sessionStorage` keys (`conway-callout-order` etc.)
- `SchedulingParams.SetupAcknowledged` already covers the setup card

---

## 8. API Contract

None — all content ships in `app/docs.html`.

---

## 9. Out of Scope

- Interactive click-through tour libraries
- Video embeds
- Per-user server-persisted onboarding state

---

## 10. Open Questions

None — defaults confirmed with the product owner during the audit.

---

## 11. Decision Record

### Decision 1: The Guide gains a persona; the docs gain a panel — no new IA shell

**Context:** A separate "Docs" tab would compete with the Guide and fragment
entry points.

**Decision:** The Guide modal stays the single entry for "how do I use this":
it gains the Planning Manager persona (routing steps), and each step can open
the docs panel at an anchor. The panel is one overlay shipped offline.

**Alternatives considered:**
- Wiki-style separate docs view — rejected: fragments help content and
  duplicates navigation.
- External wiki link — rejected: requires network, leaves the app.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Planning loop documented in-app | no | yes | panel sections |
| Warning states with a fix + link | partial | all 3 named warnings | in-browser |
