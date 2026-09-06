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

### Story 4: Choose a model with informed assumptions

**As a** manager preparing a plan or reviewing delivery
**I want** concepts, calculations, worked examples and situation-based guidance
**So that** I choose settings consistent with my estimates and evidence

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

**AC 4.1: Understand before choosing**

> Given a manager comparing planning settings
> When they read the assumptions reference
> Then each option explains its inputs, units, effect and appropriate situation,
> and simple worked examples distinguish effort, calendar duration and capacity.

**AC 4.2: Distinguish models and evidence**

> Given a manager reviewing Measure or Execution
> When they consult the manual
> Then the guide identifies the source, calculation and limits of each metric,
> including synthetic evidence and known limitations that affect interpretation.

**AC 4.3: Standard navigation stays usable**

> Given an existing contextual help link
> When the reorganized manual opens
> Then its target exists and the reader can navigate tutorials, task recipes,
> conceptual explanations and reference material without an external service.

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The Guide modal MUST gain a Planning Manager persona whose steps navigate on click | MUST |
| FR-002 | A docs panel (theme-styled overlay, content shipped in `app/docs.html`) MUST open from the Guide and from setup-card "learn more" links | MUST |
| FR-003 | Warnings (missing pods, no period start, beyond-horizon) MUST carry "learn more" deep links into the panel | MUST (beyond-horizon gets its own link: it fires alongside no-dates only when the plan also lacks a period start, so the two banners are not interchangeable) |
| FR-004 | First-visit callouts on Order and Timeline MUST be dismissible and session-persistent | SHOULD |
| FR-005 | The panel MUST cover: the planning ritual, effort model & chunking, verdicts & fever chart, baselines & comparison, pins/drags/filters, WIP models & stagger | MUST |
| FR-006 | The manual MUST introduce plan, roster, snapshot, agreement, scenario, initiative, team, track, dependency and evidence before advanced options | MUST |
| FR-007 | Settings and metric explanations MUST identify inputs, units, calculation effects, situation-based guidance and limitations | MUST |
| FR-008 | The manual MUST distinguish Plan utilization, Measure load and probabilistic-model utilization; worked examples MUST state simplifying assumptions | MUST |
| FR-009 | The guide MUST distinguish current behavior, recommended practice and unresolved product limitations, without promising unimplemented corrections | MUST |
| FR-010 | Documentation MUST provide a first-plan tutorial, recurring review recipe, concept explanations and searchable reference sections with stable links | MUST |
| FR-011 | The guide MUST credit The Goal, Critical Chain, Goldratt's Rules of Flow, The Phoenix Project and The Unicorn Project, distinguishing conceptual inspiration from Conway-specific calculations | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression | suites green | `node --test`, `go test ./...` |
| NFR-002 | Panel opens instantly, no network | offline render | in-browser |
| NFR-003 | Link integrity | Every local manual anchor and contextual view mapping resolves | Static anchor scan and mapping tests |

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
- Changes to scheduling or analytics formulas in the documentation revision

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

### Decision 2: Teach concepts, tasks and calculations in one maintained manual

**Context:** The existing manual mixes task instructions, model terminology and
prescriptive claims. Managers cannot consistently tell which data drives a view,
what a setting changes or whether a number is a measurement or a model output.

**Decision:** Reorganize the offline manual into getting started, concepts,
task guides and references. Keep existing section anchors and add explicit
contextual mappings where view IDs differ from documentation IDs. Explain each
major option through purpose, calculation, a generic worked example and when to
choose it. State units and rounding; separate Plan, Measure and Execution math.
Document current model limitations beside affected outputs. Add a repository
documentation index that points to the manual rather than duplicating its prose.

**Alternatives considered:** A second complete Markdown manual would diverge
from in-app help. An external documentation platform adds hosting and runtime
dependencies. Formula-only reference material does not teach the decision.

**Consequences:** One manual remains the user-facing explanation. Source review
and independent calculation checks are necessary when formulas change. Existing
product defects remain separately tracked; clearer documentation does not repair
their behavior. This decision was recorded on 2026-09-05 before the rewrite.

The product owner also confirmed the five books named in FR-011 as inspiration.
Introduce their management ideas before the option reference and connect each
to a practical decision. Attribute the books to their authors with publisher
links; do not describe Conway's heuristic formulas as formulas supplied by the
books or imply a complete implementation of critical-chain project management.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Planning loop documented in-app | no | yes | panel sections |
| Warning states with a fix + link | partial | all 3 named warnings | in-browser |
