# Scheduling audit and Gantt integrity

**Status:** Implemented
**Author(s):** Project maintainers
**Date:** 2026-09-05
**Story/Ticket:** Scheduling integrity audit
**Sprint/Cycle:** Current

## 1. Overview

Audit imported assignments, release constraints and rendered timelines using
generic scheduling invariants. Correct confirmed defects without changing user
inputs or embedding organizational context in the product or its fixtures.

## 2. Problem

Placeholder lead cells can become fictitious shared resources. Permanently
closed release gates can retry beyond the period. Calendar handling can move a
valid future pin earlier, and split placement can lose the calendar-adjusted
start. Chart geometry and data-quality explanations need to account for every
assigned or occupied interval, including unscheduled work.

## 3. User Stories

### Story 1: Trust scheduling constraints

As a planner I want constraints applied to real resources and valid time
windows, so apparent slack and delays have reliable explanations.

### Story 2: Inspect complete timelines

As a team lead I want all scheduled intervals and unresolved inputs visible,
so missing bars do not conceal missing work or misleading dates.

## 4. Acceptance Criteria

### Story 1

- Placeholder lead values do not consume a shared person's capacity; missing
  ownership remains visible as an assumption.
- A zero-capacity lead gate returns an unscheduled result within a bounded
  release search, with its cause retained.
- A legal future pin remains a lower bound even when earlier weeks block starts.
- Split work cannot begin within a blocked-start calendar window.
- Growth across a non-working gap charges its configured ramp overhead.
- Splitting must not delay completion when legal contiguous work finishes sooner.
- Lead role limits are visible and editable; unrelated assumption edits preserve
  settings that are not represented by the form.
- Duplicate initiative identifiers are reported rather than silently replacing
  a row's work with another row's schedule.

### Story 2

- Each rendered team-week matches the server's occupied lanes for the same work.
- Declared dependencies that cannot be resolved are explained instead of
  disappearing silently.
- A successor of unschedulable work cannot acquire a promise based on a fictitious
  predecessor finish.
- Dates, filters and exported sheets distinguish planned work, held work and
  non-working intervals.

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | Only real named leads MUST consume named resource capacity. | MUST |
| FR-002 | Every release retry path MUST terminate at the planning bound. | MUST |
| FR-003 | Pins and dependency-ready times MUST remain start lower bounds. | MUST |
| FR-004 | Calendar rules MUST apply equally to split and contiguous work. | MUST |
| FR-005 | Split ramp costs MUST account for actual lane growth across gaps. | MUST |
| FR-006 | Ambiguous initiative identities MUST produce an explicit input error. | MUST |
| FR-007 | Unresolved dependency evidence MUST remain visible. | MUST |
| FR-008 | Gantt geometry MUST preserve each work interval's occupancy and dates. | MUST |
| FR-009 | Lead capacity limits MUST be inspectable and preserved across unrelated edits. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Generic product | No organizational names or source data in committed additions | Privacy scan and review |
| NFR-002 | Regression safety | Relevant Go and JavaScript suites pass | Product checks |
| NFR-003 | Input isolation | Audit never saves or rewrites user plans | Read-only replay |

## 7. Data Model

Keep current persisted inputs. Derived warnings may describe unavailable or
ambiguous evidence. Preserve original lead display values while using canonical
resource identities for capacity accounting.

## 8. API Contract

Existing preview and schedule endpoints retain their purpose. Invalid ambiguous
inputs must receive a clear validation response, including the conflicting name.

## 9. Out of Scope

A new optimizer, individual work assignment, changing business priorities,
changing user data, publishing private fixtures or pushing commits.

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Which placeholder spellings are safely identifiable? | Maintainers | 2026-09-05 | Decision 1 uses an exact, conservative set. |

## 11. Decision Record

### Decision 1: Distinguish lead identity from ownership evidence

Treat blank, `TBD`, `None`, `N/A`, `NA`, `Not Required`, `Unknown` and `-`
as non-person values after trimming and case normalization. Do not guess that a
longer label containing these words is a placeholder. Normalize actual resource
keys by case and whitespace while retaining the supplied display text. Report
placeholder ownership as an assumption; do not silently invent a person or
remove capacity limits for genuinely named leads.

### Decision 2: Bound every release retry

Test the horizon before continuing a refused release. A permanently closed gate
must yield an unscheduled result with its binding cause rather than loop forever.
Carryover remains exempt from release gates because it is already running.

### Decision 3: Preserve lower bounds and actual phase starts

Apply start-window restrictions to the later of readiness and the saved pin.
When split placement waits past that bound, use its first occupied phase as the
actual start. Capacity gaps do not erase the last allocated lane width when
deciding whether renewed work needs a growth ramp.
Compare the split candidate with legal contiguous placement and prefer the
earlier finish; ties use the simpler contiguous allocation. This implements the
existing splitting-cost tradeoff rather than charging overhead for waiting alone.

### Decision 4: Reject ambiguous identities and qualify unresolved dependencies

Names are the current initiative identifiers. Reject blank or duplicate normalized names
at import/save and schedule boundaries rather than merging or dropping work.
Unresolved dependency references remain non-blocking under the existing model,
but must add an explicit assumption and qualify the affected forecast as
provisional. Existing cycle-breaking behavior retains its named-edge explanation.
An unschedulable predecessor has no usable completion promise; its successors
remain unscheduled. Provisional evidence propagates through valid precedence.

### Decision 5: Expose and preserve scheduling policy

Show the live default concurrent-initiative limits by role (PM 2, engineering 2,
architecture 3, program management 4), support explicit zero, and explain that
blank restores the default. Merge form-controlled values with the saved policy
while deliberately removing cleared form fields; never erase hidden settings.
Describe lead-bound work by role and capacity, without blaming individuals.
Ignore stale simulation responses after switching plans or changing inputs,
using the same request ownership principle already used by other plan views.
Apply this ownership guard to upload previews as well, including replacement
previews and discarding a draft before its response arrives.

### Decision 6: Do not reward omitted commitments

Compare weighted unstarted work before weighted lateness when choosing an
ordering. A zero lateness score caused by omitting work is not an improvement
over delivering it. Keep coverage cost and lateness separate and visible;
do not fabricate dates for held work. Reuse existing initiative weights and
date-lock dominance. Apply the same comparison wherever candidate schedules are
selected, including WIP model recommendations.

### Decision 7: Make chart context complete and editable

Repeat a readable time axis on team cards and include it in exports. Apply
calendar, today and period markers to team tracks as well as portfolio rows.
Reduce tick density on narrow screens. Work beginning beyond the selected span
gets a labeled outside-view state instead of a zero-width bar; its dates remain
available in the sheet. Exports must preserve held initiative labels when
interactive controls are removed. Provide a keyboard-operable team-sheet button.
Zero-capacity rows obey the same work filter as physical-lane rows. Clamp a left
resize at week zero before calculating both its new start and anchored effort.
If legal saved lane pins require flexible work to change lanes over time, split
its visual intervals at reservation boundaries instead of silently overlapping
bars; this is layout only and must conserve server occupancy.
For flat slices interrupted by calendars or completion waits, use the schedule's
per-week initiative occupancy to derive visible work intervals. Keep the overall
start/finish promise separate from weeks of active work; do not draw holidays as
occupied lanes or duplicate calendar policy in the browser.

### Decision 8: Validate saved lanes against the edited schedule

Validate every saved lane pin after any scheduling edit, including horizontal
moves and estimate changes. Retain all start pins, apply the plan's actual loss
and horizon, and inspect authoritative occupied weeks. Fixed lane reservations
must not overlap each other or exceed physical width. Unpinned work remains
flexible rather than being assumed to occupy lane zero. Calendar gaps and
completion waits reserve no lanes. Use the same phase-width offset clamping as
the chart for growth; a pin identifies the initial lane, not extra capacity.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Confirmed invariant violations | Zero in accepted regression cases | Zero | Independent tests and replay |
| Undisclosed missing input evidence | Reproduced | Zero in regression cases | Warnings and forecast state |
| Rendered occupancy disagreement | Zero across 910 visible team-weeks | Zero | Browser geometry accounting |

## Review Checklist

- Decisions precede implementation.
- Test author and implementation author remain separate.
- Confirm findings independently before changing behavior.
- Report residual limits without claiming universal algorithm correctness.
