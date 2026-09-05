# Scheduling and timeline audit

Audit date: 2026-09-05. Decision record:
[Scheduling audit and Gantt integrity](../specs/019-scheduling-audit-and-gantt-integrity.md).

## Scope and findings

A read-only replay covered a 29-initiative, 35-team reference plan with 203
team assignments. Source names and descriptions were replaced in the audit
input. The workbook, full replay input and private identifiers are not committed.
The audit did not change saved plan data or scheduling policy.

| Confirmed defect | Correction and regression coverage |
|---|---|
| Placeholder owners consumed shared lead capacity; spelling variants split real identities. | Exact placeholder values do not represent people. Real lead keys normalize case and whitespace, with missing ownership disclosed. |
| A permanently closed lead gate could keep retrying. | All release retries terminate at the scheduling bound and retain the holding reason. |
| Calendar processing could move future pins earlier or start waiting split work during a freeze. | Pins remain lower bounds; placement rechecks actual starts. |
| Lane growth after a capacity gap lost ramp overhead; splitting could finish later than contiguous placement. | Growth preserves prior width and pays the configured tax. Placement compares both legal candidates. |
| Duplicate names could alias work; unresolved dependencies disappeared; held predecessors could imply usable dates. | Input boundaries reject ambiguous names. Missing evidence qualifies forecasts, and unschedulable predecessors hold successors. |
| Ordering could prefer a lower lateness score obtained by leaving work unstarted. | Weighted unstarted work is compared before weighted lateness throughout ordering, recommendations and remedies. Both metrics remain visible. |
| Assumption edits could discard hidden policy; late preview or simulation responses could replace newer state. | Forms preserve unrelated policy. Async responses must still own the current plan, input epoch and request. |
| Saved lane checks used inaccurate placement assumptions. | Validation uses the edited schedule, actual capacity loss, preserved starts and all fixed reservations; unpinned work remains flexible. |
| Flat bars could fill calendar gaps; flexible lanes could overlap complementary saved pins. | Team bars follow authoritative occupied weeks and split visual intervals around reservations. API-omitted empty initiative lists are treated as idle weeks. |
| Team charts lacked local date context and could hide work outside the viewport or lose held names in PNG exports. | Repeated responsive axes, calendar markers, outside-view notices, held-work explanations and accessible team-sheet controls preserve context. |
| Resizing left past week zero changed anchored effort incorrectly. | The effective drag distance is clamped before computing dates and effort. |

## What the reference replay shows

Using its current 26-week horizon, effort estimates, 10% capacity loss and 90%
target utilization, the replay still leaves 14 initiatives unstarted within the
period: 13 held by lead capacity and one by the drum release policy. These are
reported constraints, not missing chart bars. Idle team lanes do not by themselves
prove that an initiative can pass shared release gates.

The input contains eight nonblank placeholder lead cells and four self-referencing
team dependencies, as well as references to teams without corresponding work in
the same initiative. Those assumptions now remain visible. Planners should review
the declared dependencies and ownership evidence before treating provisional
dates as commitments.

All 108 placed, estimated team slices supplied at least their required productive
effort after capacity loss; one additional placed slice had an unknown estimate.
All 2,415 emitted forecast team-weeks stayed within physical lane capacity.
The browser matched all 910 team-week cells in the visible 26-week span to server
occupancy, with no overlapping bars on an individual lane. Twelve team cards
contained explicitly labeled work beyond the selected span.

## Execution evidence

Checks executed on 2026-09-05:

- `go test -race ./...`: all tested packages returned `ok`; the server package
  completed in 6.828s and planning in 4.146s.
- `node --test tests/*.test.mjs`: `tests 358`, `pass 358`, `fail 0`.
- `golangci-lint run ./...`: `0 issues.`
- A headless browser loaded the real chart modules with the sanitized replay:
  `teamWeekChecks: 910`, `failures: []`, `exported: true`,
  `mobileVisibleTicks: 5`, `pageErrors: []`. Desktop, mobile and downloaded PNG
  output were inspected.
- Twelve targeted JavaScript regressions failed against the earlier committed
  chart, drag and async-response implementations. The API-shaped idle-week case
  also reproduced a phantom occupied week before its correction.

The generic regression cases live in `server/planning/schedule_audit_test.go`,
`server/planning/lanepins_audit_test.go`,
`server/scheduling_integrity_handlers_test.go`, `tests/scheduling-audit.test.mjs`,
`tests/timeline-correctness.test.mjs` and `tests/drag.test.mjs`. Existing suites
also cover capacity, splitting, report wording and remedy comparisons.

## Remaining limits and opportunities

This is a constraint-aware heuristic, not a proof of a globally optimal schedule.
Missing dependency evidence remains non-blocking under the existing model and
therefore produces provisional forecasts. Unknown estimates need planner input.
Changing lead limits or release policy is a planning decision, not an automatic
repair for idle team capacity.

Portfolio start-to-finish bars represent elapsed promise spans; team lanes show
occupied work intervals. Dependency names are inspectable, but cross-row visual
dependency connectors remain a possible future improvement. The reference replay
cannot exercise every calendar or carryover combination; small generic regression
fixtures cover these edge cases separately.
