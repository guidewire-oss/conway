# Bootstrap Adoption Debt

**Status:** Implemented
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-26
**Story/Ticket:** audit follow-up, post spec-010
**Sprint/Cycle:** n/a

---

## 1. Overview

Bootstrap 5.3 is vendored and used for modals, dropdowns, tooltips, form
classes and theming. The September follow-up completes generic component
adoption and replaces duplicated layouts in Next work and execution reviews.
Planning calculations, saved context and action semantics remain unchanged.

---

## 2. Problem

The design-system PRs (#21–24) predate Bootstrap: a hand-rolled token layer
came first, and Bootstrap was adopted *underneath* it (vendored, tokens
bridged) to avoid rewriting every view at once. The migration stopped at the
bridge. The following is the historical audit baseline, not a current inventory:

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

**AC 5.1:** Generic action buttons, cards, badges and form controls use
Bootstrap classes and states. Application hooks may remain as classes, but
must not recreate their framework component's base styling.

**AC 5.2:** Next work and execution review controls, forms and summaries use
Bootstrap layout and spacing utilities. At 360px, long content wraps and
wide evidence tables scroll within their container. Normal-size control and
badge text meets 4.5:1 contrast in both themes.

**AC 5.3:** Dynamic renders retain framework classes. Keyboard activation,
visible focus, disabled actions, selected group state, modal dismissal and
menu navigation and Bootstrap sizing modifiers remain usable; domain label
geometry is preserved without duplicate state controllers.

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | Migrate all form controls to BS form classes, delete the scattered input CSS overrides | DONE — forms.js injector (2026-08-26) |
| FR-002 | Migrate tooltips to BS Tooltip (delegated init); remove the custom tip element | Implemented; native titles retained on dense chart marks |
| FR-003 | Migrate `.seg` groups to BS button groups | Implemented with state-owner ARIA updates |
| FR-004 | Resolve the `.card` shadow — ours renamed to `.panel-card` (32 class usages: 11 in index.html, 21 across the view modules, plus the CSS rule) | DONE (2026-08-26) |
| FR-005 | No visual regressions beyond BS-native focus/validation states | MUST |
| FR-006 | Generic buttons, cards, badges and forms use framework primitives | MUST |
| FR-007 | Recent operational screens share framework layouts on desktop and mobile | MUST |

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

- Unrelated visualization geometry and pixel-for-pixel utility conversion of
  every legacy layout. Generic component adoption is in scope throughout the app.
- Replacing the custom Gantt/timeline rendering (not a BS component domain)

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Keep native `title` on dense chart bars (cheap, browser-native) or migrate to BS Tooltip too? | Anoop | 2026-09-02 | Resolved: keep native on bars, Bootstrap Tooltip for UI chrome |

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

### Decision 2: Complete primitive adoption before further feature work

**Context:** The maintainer requested Bootstrap-first implementation before
next feature work on 2026-09-06. Theme-compatible custom buttons and cards
still duplicate framework behavior. New operational layouts add avoidable CSS.

**Decision:** Put framework classes directly in owned templates; retain the
existing form adoption bridge for dynamic/legacy callers. Delete duplicated
base input, button and card styling; express brand changes through Bootstrap
variables. Preserve selectors used by app logic and migrate their visuals.
Use Bootstrap utilities and responsive layout for Next work, execution and
weekly review. Keep native disclosures and custom domain visualizations.
Group labels and selected states belong to the existing state owner, not a
second Bootstrap toggle controller. Test real interactions in the browser.

**Alternatives considered:** A broad runtime component injector would hide
missing template adoption and complicate state ownership. A full rewrite of
visualization layouts would create unrelated rendering risk. Both are rejected.
This decision expands the original four-component migration scope.

**Consequences:** Templates and CSS change together; existing workflow tests
must still pass. Component guidance and durable agent instructions change in
the same increment. No library version change is required.

### Decision 3: Preserve contextual sizing and readable evidence

**Context:** Framework defaults can override compact selectors, status text,
and the guide's editorial spacing even when controls adopt the right classes.

**Decision:** Adopt framework primitives while preserving the control's context:
embedded selectors stay compact, evidence remains readable, and state and focus
remain accessible across themes. Use framework utilities and variables for
generic behavior; reserve scoped styling for domain geometry and editorial
reading density. Keep browser acceptance independently executable while the
Ginkgo harness owns its isolated server and lifetime.

**Alternatives considered:** Accepting all framework defaults would change
reading density and domain layout; restoring generic custom component styles
would duplicate Bootstrap. Both are rejected.

**Consequences:** Test computed geometry, contrast and focus in rendered views.
The existing theme and state owners remain authoritative; no version changes.
Neutral plan badges use theme-aware surfaces and text. Compact game controls
wrap within their cards, and calendar delete controls retain usable targets.
Help explanations retain native title fallback when tooltip initialization is
unavailable. Visibility acceptance checks rendered panels, not attributes alone.
KPI collections wrap before labels become unreadable. Navigation and accepted
ordering use the existing theme through Bootstrap component variables and
utilities. Dense table actions and inline import fields retain compact sizing;
domain inspector padding remains responsive rather than overridden by utilities.
All neutral metadata follows the theme, and related semantic token families stay
consistent. Glossary and contextual help share one escaped button implementation;
static help retains the same native fallback. Domain focus indicators remain
singular, icon-only actions are named, and long agreement labels can wrap.
The guidance persona is a single choice: use native radio inputs styled as
Bootstrap buttons, retain focus after changing the role, and preserve its saved
preference. A selected content filter does not require a partial tab widget.
Base table theme overrides must preserve contextual Bootstrap table variants.
Authentication fields share the theme focus tokens; inline account creation
keeps its name field compact while allowing the control group to wrap.
Quality-card acceptance covers both complete and unmatched-team evidence.
Plan setup settings wrap within narrow screens, and account expiry actions use
compact controls without losing native date input or existing action handlers.
What-if lever selectors keep their type, target and action grouped at desktop
widths and wrap within the available mobile space. Link RGB aliases derive from
the primary token so opacity utilities follow a customized palette.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Ready queue and execution actions missing Bootstrap button classes | 0 in rendered acceptance fixtures | 0; regression guard | `#ready button:not(.btn), #evidence button:not(.btn)` count in `tests/browser/bootstrap-adoption.mjs` |
| Tooltip systems | Bootstrap + native chart titles | Unchanged | code |
| Operational workflow regression | Existing acceptance suite | Pass in both themes and at 360px | Ginkgo/Playwright |

The class-coverage count is a regression guard over the named rendered fixtures,
not a claim that every scoped application style has been removed. A count above
zero fails acceptance even when the previous run met the target. The component
registry governs review of controls outside these fixtures.
