# Readable scrollable timelines

**Status:** Done
**Author(s):** Conway maintainers
**Date:** 2026-09-10
**Story/Ticket:** PR #88 follow-up
**Sprint/Cycle:** Planning usability

## 1. Overview

Managers can inspect long schedules without compressing work into unreadable
bars. Both timeline lenses scroll horizontally, and initiative names can be
revealed by widening their label column without changing the schedule.

## 2. Problem

Fitting 154 weeks into one card crowds dates and hides short work. Independent
row sizing can also make work appear to start at different dates across lanes.
Truncated initiative names are difficult to distinguish during plan reviews.

## 3. User Stories

### Story 1: Read a long schedule

**As a** manager, **I want** aligned dates and horizontally scrollable work,
**So that** I can follow a team's work across the entire selected span.

### Story 2: Reveal initiative names

**As a** planner, **I want** to resize the initiative label column,
**So that** I can distinguish full names while retaining timeline context.

## 4. Acceptance Criteria

**AC 1.1:** Given a 154-week schedule with busy and idle lanes, when viewing
either lens, then equal weeks share equal horizontal coordinates and the
selected span remains reachable through a horizontal scrollbar.

**AC 1.2:** Given a scrolled chart, when inspecting rows or exporting a team,
then row labels stay visible and the PNG includes the entire selected span.

**AC 2.1:** Given the default label width, when dragging its divider right or
using arrow keys on the focused divider, then labels widen and dates, bars,
buffers and markers remain aligned. Home restores the default width.

**AC 2.2:** Given a resized or scrolled chart, when selecting an initiative or
switching lenses, then the width preference survives; selecting work preserves
the scroll position. Narrow screens retain a usable chart without page overflow.

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-001 | Both lenses MUST show the selected span without misleading bar geometry. | MUST |
| FR-002 | Users MUST be able to scroll long timelines horizontally. | MUST |
| FR-003 | Initiative labels MUST support pointer and keyboard resizing with bounded widths. | MUST |
| FR-004 | Existing selection, filtering, precise edits and PNG exports MUST remain usable. | MUST |

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|-------------|-----------|---------------|
| NFR-001 | Geometry accuracy | Within 1 CSS pixel of schedule intervals | Playwright bounding boxes |
| NFR-002 | Responsive layout | No document overflow at 360px | Playwright |
| NFR-003 | Accessibility | Named focusable separator, keyboard controls and visible focus | Playwright |

## 7. Data Model

No persisted planning data changes. Label width is a presentation preference
for the current plan session; it does not affect capacity or dates.

## 8. API Contract

Existing schedule and export behavior; no new API.

## 9. Out of Scope

Scheduler policy changes, cross-device preferences and resizable detail forms.

## 10. Open Questions

None; the existing default label and initiative detail sizes remain unchanged.

## 11. Decision Record

### Decision 1: Shared canvas geometry and native scrolling

Use a minimum 24 CSS pixels per week inside a native horizontal scroll viewport.
Axes, overlays and every track share the same label offset and time width.
Labels remain sticky; a pointer/keyboard separator changes only label width.
Keep the existing 160px default (96px on narrow cards), bounded by available
viewport space. This supersedes spec 001 FR-035's fit-only display rule.
Retain percentage-based schedule coordinates within the larger canvas.
Remove decorative minimum bar widths that can exaggerate occupied intervals.
Exports expand to the full canvas rather than capturing a cropped viewport.
Zero-duration scheduled slices render as named checkpoints in a separate team
row, not occupied tracks; zero-duration initiatives use a checkpoint marker.
Their start week remains accurate and filtering still applies. Browser and unit
coverage include a checkpoint at week 53 in a 154-week span.
Bootstrap supplies standard controls and layout; custom CSS supplies chart
geometry and the separator because Bootstrap has no resizable Gantt component.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| 154-week chart | Compressed labels and work | Readable, aligned and scrollable | Browser regression |
| Label width | Fixed | Pointer and keyboard adjustable | Browser regression |

## Review Checklist

- [x] Problem, user stories and acceptance criteria recorded
- [x] Edge cases, accessibility and responsive requirements recorded
- [x] Scope and technical decisions separated
- [x] Browser regression and export checks pass
