# Measure source context and associations

**Status:** In Progress
**Author(s):** Project maintainers
**Date:** 2026-09-05
**Story/Ticket:** Make analytics provenance and switching explicit
**Sprint/Cycle:** Current

## 1. Overview

Every Measure screen identifies the snapshot and roster supplying its data,
explains what that screen measures, and offers source switching and association
controls. Planning inputs and execution evidence remain distinct.

## 2. Problem

The snapshot picker disappears when only one snapshot exists. Network, WIP,
quality and simulation screens then show numbers without a visible source name,
capture date or relationship to the open plan. Example simulator tasks can also
look like imported delivery evidence.

## 3. User Stories

### Story 1: Identify and change the measured organization

As a planner I want visible source metadata and a snapshot selector on Measure
so I can recognize the organization, time and roster behind the numbers.

### Story 2: Associate evidence deliberately

As a planner I want direct access to roster associations and clear execution
instructions so I can connect plan initiatives to Jira evidence intentionally.

## 4. Acceptance Criteria

**AC 1.1:** Given one snapshot, when any Measure view opens, then its name,
source kind, capture time, project scope and associated roster remain visible.

**AC 1.2:** Given multiple snapshots, when another is selected, then all Measure
data reloads from that snapshot and the current Measure view is retained.

**AC 1.3:** Given empty or unavailable metadata, when the context is rendered,
then it describes the missing evidence and provides recovery rather than naming
an unrelated source. Example data and synthetic estimates are explicitly labeled.

**AC 2.1:** Given manager access, when the context is displayed, then import,
snapshot/roster association management and plan navigation are directly available.
Read-only users retain source selection without unauthorized edit controls.

**AC 2.2:** Given Measure data, when the user seeks a plan association, then the
interface explains Execution snapshot selection, epic bindings and agreed baselines.
Changing Measure's selection does not rewrite plan inputs or execution selection.

**AC 2.3:** Given the Feature Simulator, when example tasks, an imported epic or
an edited scenario is shown, then its task origin is labeled independently of
the snapshot providing team statistics.

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | Measure MUST expose source context even with one snapshot. | MUST |
| FR-002 | Source metadata MUST distinguish imported observations, examples, synthetic statistics and unavailable evidence. | MUST |
| FR-003 | Source changes MUST affect the Measure views consistently. | MUST |
| FR-004 | Authorized association controls MUST be discoverable alongside source context. | MUST |
| FR-005 | Plan and simulation inputs MUST be distinguishable from snapshot evidence. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Accessible and responsive controls | Labeled keyboard controls; no horizontal overflow at 390px | Browser checks |
| NFR-002 | Metadata safety | Escape names and IDs; no metadata-driven HTML execution | Regression tests |
| NFR-003 | Data preservation | No plan or snapshot mutation from viewing or switching | Request inspection |

## 7. Data Model

Reuse snapshot metadata, roster metadata and loaded team statistics. Simulator
task-origin state belongs to the current scenario and is not a plan association.

## 8. API Contract

Reuse existing snapshot and roster read APIs and existing management dialogs.
No new persistence API or automatic association is introduced.

## 9. Out of Scope

Automatic Jira-to-initiative matching, altering saved plans or rosters, new
analytics algorithms, and external publishing.

## 10. Open Questions

None.

## 11. Decision Record

### Decision 1: Keep one persistent Measure context panel

Place a shared panel immediately before the active view. It remains visible on
Home, Measure and snapshot-based organization what-ifs; it hides on Plan and
Game views, which own separate inputs. Include a view-specific explanation and
a snapshot selector even when only one snapshot exists. Label missing dates,
scope and roster metadata explicitly. Synthetic statistics are counted from the
actual loaded team state, independently of the snapshot's source label.

### Decision 2: Reuse existing switching and association contracts

Switch snapshots by preserving the current URL and changing only its snapshot
parameter, then reloading all derived views. Escape metadata before generating
HTML. Managers get direct import, snapshot associations and roster actions;
read-only users can select readable snapshots. Failed catalog or roster reads
remain distinguishable from an empty collection and offer retry.
Successful snapshot or roster edits notify the context to refresh metadata
without resetting local scenarios. Unknown association metadata must not be
reported as a confirmed missing association.

### Decision 3: Explain the plan bridge and simulator task origin

State that Measure reads a dated capture; Excel plans hold separate assumptions.
Direct planners to a plan's Execution view to select evidence, confirm epic keys
and compare with the saved agreement. Show simulator origin as example tasks,
imported epic or edited scenario. Editing rows marks forecasts stale until run;
switching snapshots reloads the simulator and resets its local scenario, which
the switching control explains.
Imported tasks with an absent or unrecognized team retain that missing evidence;
they do not silently acquire the first roster team or a forecast based on it.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Measure views with visible source identity | Picker hidden with one source | All source-backed views | Browser navigation |
| Ambiguous simulator task origin | No persistent label | Zero tested origin transitions | Scenario tests |

## Review Checklist

- Preserve existing authorization and source selection semantics.
- Keep unknown evidence explicit.
- Inspect desktop/mobile layouts and source switching.
