# Bootstrap Adoption Debt

**Status:** In Progress
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-26
**Story/Ticket:** audit follow-up, post spec-010
**Sprint/Cycle:** n/a

---

## 1. Overview

Bootstrap 5.3 is vendored and genuinely used for modals, dropdowns, theming
and the variable bridge (119 `--bs-*` mappings in conway.css). But the views
still hand-roll components Bootstrap ships: 47 custom input styles, a bespoke
tooltip system, custom segmented controls, a shadowing `.card`, and zero use
of BS utility classes. This spec records the debt and sequences its payoff —
no behaviour changes, only component substitution.

---

## 2. Problem

The design-system PRs (#21–24) predates Bootstrap: a hand-rolled token layer
came first, and Bootstrap was adopted *underneath* it (vendored, tokens
bridged) to avoid rewriting every view at once. The migration stopped at the
bridge. Consequences today:

- 47 `background: var(--panel2)` input overrides fight theme states BS
  already solves (focus rings, sizing, validation states, dark-mode
  correctness)
- A bespoke tooltip div in main.js duplicates BS Tooltip (hover + focus +
  delay, hand-tuned) and coexists with native `title` attributes — two
  tooltip systems plus the native one
- `.seg` segmented controls lack the keyboard/ARIA semantics BS `btn-group`
  provides
- A custom `.card` class shadows BS's card — possible double-styling
- No BS utilities (`d-flex`, `gap-*`, `mb-*`): all spacing hand-CSS

---

## 3. User Stories

### Story 1: Native form semantics

**As a** planner using keyboard or assistive tech
**I want** inputs, selects and checkboxes with Bootstrap's focus/ARIA states
**So that** the app behaves like every other well-built form

### Story 2: One tooltip system

**As a** any user
**I want** one consistent tooltip behavior everywhere
**So that** hover timing and focus behavior never surprise

---

## 4. Acceptance Criteria

**AC 1.1:** All text/number/date inputs and selects use `form-control`/
`form-select` (or `form-check` for checkboxes); the custom input CSS block is
deleted; focus rings and dark-theme correctness come from BS

**AC 2.1:** The custom tooltip div is removed; glossary `?` affordances and
`data-tip` elements use BS Tooltip with delegated initialization; native
`title` attributes are migrated or intentionally kept (e.g. bar titles) with
the split documented

**AC 3.1:** `.seg` controls become `btn-group`/`btn` (or `nav-pills`) with
BS's active/ARIA semantics; the custom `.seg` CSS is deleted

**AC 4.1:** The custom `.card` is renamed or reconciled with BS `card`;
no shadowing

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | Migrate all form controls to BS form classes, delete the scattered input CSS overrides | DONE — forms.js injector (2026-08-26) |
| FR-002 | Migrate tooltips to BS Tooltip (delegated init); remove the custom tip element | REMAINING — riskiest (hundreds of affordances), next slice |
| FR-003 | Migrate `.seg` groups to BS button groups | REMAINING — 12 call sites, ARIA wiring needed |
| FR-004 | Resolve the `.card` shadow — ours renamed to `.panel-card` (22 usages + CSS rule) | DONE (2026-08-26) |
| FR-005 | No visual regressions beyond BS-native focus/validation states | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression | suites green + visual spot-check | `node --test`, in-browser |
| NFR-002 | Tooltip init cost on 29-row plans | imperceptible (< 50ms) | in-browser |

---

## 7. Data Model

None.

---

## 8. API Contract

None.

---

## 9. Out of Scope

- Utility-class migration for layout (d-flex/gap-* everywhere) — cosmetic,
  churn-heavy, no behavior gain
- Replacing the custom Gantt/timeline rendering (not a BS component domain)

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Keep native `title` on dense chart bars (cheap, browser-native) or migrate to BS Tooltip too? | Anoop | 2026-09-02 | pending — default: keep native on bars, BS Tooltip for UI chrome |

---

## 11. Decision Record

### Decision 1: Sequenced substitution, bridge kept

**Context:** The token bridge (conway.css) is the load-bearing piece — it is
what makes BS components adopt the app's theme. It works; do not touch it.

**Decision:** Migrate components INTO the bridged BS, in the order
inputs → tooltips → segs → card-shadow. The bridge stays the single theming
seam.

**Alternatives considered:**
- Full rewrite to BS classes everywhere — rejected: churn without behavior
  gain outside the four components named.
- Drop Bootstrap and finish the hand-rolled system — rejected: we would be
  re-implementing modals/dropdowns/tooltips that already work.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Custom input overrides | 47 | 0 | grep |
| Tooltip systems | 2 + native | 1 (+ native on bars per Q1) | code |
| Custom `.seg` CSS | 3 rules + JS wiring | 0 | grep |
