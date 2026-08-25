# Spec-001 Cleanup: Pin Control, Fever Chart, Pod-View Export, Idle Attribution, Drum Stagger

**Status:** In Progress
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-25
**Story/Ticket:** closes the remaining gaps of specs/001-plan-execution-order.md
**Sprint/Cycle:** n/a

---

## 1. Overview

Five requirements in spec 001 shipped as data-without-a-face or not at all:
the server already carries `priorityLocked` but the reconciliation view offers
no way to set it; buffer weeks are computed and displayed per initiative but
there is no fever chart of their consumption; the per-pod view cannot be taken
into a meeting; the aggregate-consistency guarantee (mean weekly utilization ≈
flat ρ) is enforced by tests but any divergence is not *attributed* to the
planner; and the drum stagger (`targetUtilization`) is a parsed, documented
field that nothing reads. This spec closes those five gaps.

---

## 2. Problem

- A planner who reads "the engine moved this initiative from rank 2 to rank 5"
  has no next action in the UI: pinning the priority (AC 6.2 of spec 001)
  requires hand-editing the initiative. The control exists server-side only.
- Buffer size is shown once, statically. A planner cannot see whether an
  initiative is *consuming* its buffer faster than the chain burns (FR-024),
  which is the early-warning signal the Observe fever chart already provides
  for the live org.
- The per-pod view is a screen, not an artefact: FR-043 says it SHOULD be
  exportable for a team meeting; today a pod lead screenshots the browser.
- NFR-005 / AC 4.2 promise that where mean weekly utilization diverges from
  the flat ρ, "the difference is attributable to reported idle time" — the
  attribution sentence is not rendered anywhere.
- The drum stagger is specified (§7 `targetUtilization`, Decision 5), parsed,
  and commented — but `grep` finds no reader. A planner who set it would get
  silence, which is the worst failure mode for a planning tool (Decision 3:
  an unexplained number reads as a bug).

---

## 3. User Stories

### Story 1: Pin a priority from the reconciliation view

**As a** planning manager
**I want** a "pin this priority" control on each rank-deviation row, and an
"unpin" on each locked row
**So that** I can force my stated order where I disagree with the engine,
without leaving the Order view

### Story 2: Watch buffer consumption

**As a** planning manager
**I want** a fever chart per initiative: buffer burn (share of buffer weeks
consumed by elapsed chain weeks) against the buffered commit week
**So that** I see which initiatives are eating their safety margin early

### Story 3: Take the pod view into a meeting

**As a** pod lead
**I want** one button that exports my pod's timeline as a PNG
**So that** I can paste it into a deck or chat without screenshots

### Story 4: Understand a utilization gap

**As a** planning manager
**I want** the Order view to state, per pod, where mean weekly utilization
differs from flat ρ and how much of the gap is idle time (WIP-gated,
calendar-blocked, or dependency-wait)
**So that** the two views' disagreement is an explanation, not an apparent bug

### Story 5: Stagger releases at the drum

**As a** planning manager
**I want** a `targetUtilization` knob that holds planned load at the drum pods
below the target, with each held release's reason named
**So that** the constraint is not scheduled to 100% and every late start is
explained

---

## 4. Acceptance Criteria

### Story 1: Pin a priority

**AC 1.1: Pin from a deviation row**

> Given a computed order whose proposed order differs from the stated order
> When the planner activates "pin this priority" on a deviation row
> Then the initiative's `priorityLocked` is persisted and the order is recomputed
> And the row shows the locked state

**AC 1.2: Unpin restores the proposal**

> Given a locked initiative
> When the planner unpins it
> Then the lock is cleared and the engine's proposal for that initiative
> returns, recomputed

**AC 1.3: Pin everything degrades gracefully**

> Given every initiative pinned
> When the order is computed
> Then the schedule follows the stated order and the reconciliation view
> reports no priority-change proposals (inherits spec 001 AC 6.3, which
> already holds server-side)

### Story 2: Fever chart

**AC 2.1: The fever point is computed per initiative at its target week**

> Given a scheduled initiative with a buffer of B weeks, a raw chain from start
> to finish, and a target week T
> When the fever chart is rendered
> Then progress = (T − start)/(finish − start) clamped to 0..1, burn =
> (commit − T)/B when T precedes the commit week (else 0), and the burn ratio
> burn/progress zones the point exactly as the Observe fever chart zones

**AC 2.2: Verdict zones reuse the existing thresholds**

> Given the app's red/amber/green utilization semantics
> When burn is displayed
> Then <60% is green, 60–90% amber, >90% red at the commit week, consistent
> with FR-044's non-color-only rule (position and label carry meaning too)

### Story 3: Export the pod view

**AC 3.1: One button, one file**

> Given the per-pod view is open and rendered
> When the pod lead activates "Export PNG"
> Then a PNG of the pod timeline (lanes, slices, labels, axis) downloads, with
> the current theme's colors

### Story 4: Idle attribution

**AC 4.1: The gap is stated per pod**

> Given a pod whose mean weekly utilization differs from its flat ρ by more
> than 2 percentage points
> When the Order view renders the pod
> Then a sentence reports both numbers and the idle weeks attributable to WIP
> gating, calendar windows, and dependency waits

**AC 4.2: No gap, no sentence**

> Given a pod within 2 percentage points
> When the Order view renders
> Then no attribution sentence appears (NFR-005's silence-when-consistent)

### Story 5: Drum stagger

**AC 5.1: The knob does something**

> Given `targetUtilization` set below 1 and a drum pod
> When the order is computed
> Then releases are staggered so planned drum load stays at or below the
> target, and each delayed release names the stagger as its binding reason

**AC 5.2: Default off**

> Given `targetUtilization` absent or 0
> When the order is computed
> Then releases are not staggered and the control shows its off state
> (inherited behaviour is unchanged)

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The Order view MUST offer pin/unpin of an initiative's `priorityLocked` from the reconciliation rows, persist it through the existing edit API, and recompute | MUST |
| FR-002 | The system MUST compute per-initiative buffer burn as elapsed chain weeks over buffer weeks and expose it in the schedule payload | MUST |
| FR-003 | The Order view MUST render a fever-chart view (burn trend vs reference) per initiative, reusing the app's red/amber/green semantics and FR-044's redundancy rule | MUST |
| FR-004 | The per-pod view MUST offer a PNG export producing the rendered timeline including lanes, labels and axis | SHOULD |
| FR-005 | The schedule payload MUST report, per pod, mean weekly utilization, flat ρ, and idle weeks bucketed by cause (WIP gate, calendar window, dependency wait) | MUST |
| FR-006 | The Order view MUST render the attribution sentence only where the gap exceeds 2 percentage points | MUST |
| FR-007 | The release rule MUST consume `targetUtilization`: hold releases so planned load at each drum pod stays at or below the target, naming the stagger in the binding reason | MUST |
| FR-008 | The assumptions form MUST offer the `targetUtilization` control only once FR-007 lands (no dead knobs — spec 001's own rule, order.js:419) | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression to existing schedules | existing Go spec suite green | `go test ./...` |
| NFR-002 | Fever chart renders for the demo plan (10 teams) without layout overflow | no horizontal scroll at 1280px | in-browser |
| NFR-003 | PNG export completes for an 8-lane pod view | < 2s | in-browser |
| NFR-004 | Existing JS test suite stays green | 175+ pass, 0 fail | `node --test` |

---

## 7. Data Model

Changes to `SchedulingParams` and the schedule payload (spec 001 §7 owns the
base entities):

- `targetUtilization`: number — now consumed (was: parsed, unread)
- Schedule payload additions:
  - per initiative: `bufferBurn`: number (0..1)
  - per pod: `meanUtil`: number, `flatRho`: number, `idleWeeks`: `{wipGate, calendar, dependency}`

---

## 8. API Contract

No new endpoints. All changes ride the existing schedule computation and edit
endpoints (`POST /api/plan/{id}/schedule`, the initiative edit API that already
persists `priorityLocked`).

---

## 9. Out of Scope

- Actuals-based fever burn (burn from *observed* progress) — Story 10, blocked
  on Jira access; this spec's burn is plan-time (elapsed chain fraction)
- Site-overlap factors — spec 003's open questions
- Changing any flat-ρ number in Network/simulator views

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should pin/unpin be a single toggle control or two buttons (spec 001 AC 6.2 says "pin this priority" only)? | Anoop | 2026-08-28 | pending — default: toggle |
| Q2 | Fever chart: one chart with all initiatives overlaid, or one per initiative row? Observe's fever chart is one line per epic on shared axes. | Anoop | 2026-08-28 | pending — default: shared axes, one line per initiative |
| Q3 | PNG export via client-side canvas render vs server-side rasterization? | Anoop | 2026-08-28 | pending — default: client-side (SVG → canvas → PNG), no new server dependency |

---

## 11. Decision Record

### Decision 1: Close spec 001's gaps in one cleanup spec, not five

**Context:** The five items were discovered at different times (pin UI during
the IA review, stagger during this session's grep, fever/export/idle from the
original spec's own TODO trail).

**Decision:** One spec, one PR series, because all five touch the same three
files (`server/planning/schedule.go`, `app/js/order.js`, spec 001's own text)
and share the verification path.

**Alternatives considered:**
- Five micro-PRs — rejected: five CI cycles and five review rounds for
  changes that share a suite and a view.

**Consequences:** The PR is reviewable story-by-story (one commit per story)
but lands atomically.

### Decision 2: Buffer burn is plan-time, not progress-time

**Context:** A true CCPM fever chart burns buffer against *completed* work.

**Decision:** Burn = elapsed chain weeks / buffer weeks on the *scheduled*
chain. It answers "if the plan runs exactly as scheduled, how much safety is
left at the commit week" — a consistency check, not a tracking report.

**Alternatives considered:**
- Waiting for Story 10 actuals — rejected: FR-024 is a SHOULD due now, and
  the plan-time view is the correct comparison for the baseline contract.

**Consequences:** The chart is labeled plan-time; the actuals-based burn is
Story 10's to add.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Spec 001 open gaps | 5 | 0 | this spec's stories done |
| Dead knobs in assumptions form | 1 (targetUtilization) | 0 | order.js:419 comment retired |
| Reconciliation rows with an action | 0 | all | in-browser |
