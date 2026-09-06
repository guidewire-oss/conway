# Audit partial evidence and request timing

Observed 2026-09-05 by running the generic probes retained in
`docs/audits/2026-09-05/` against product revision `d349e23`.

The existing JavaScript suite passed 374 tests, and an uncached server race-test
pass completed, while focused probes reproduced missing-scope completion claims,
late-response draft loss and stale agreement state. Passing helper tests does
not establish that a manager can safely interpret or save the resulting view.

For execution reviews, distinguish captured-subset evidence from evidence for
the entire agreement before deriving completion or calibration. For asynchronous
editing, test which plan and draft revision owns a response, and whether inputs
changed while it was pending. Test these boundaries directly rather than adding
more assertions about static labels.

Provenance: `server/planning/actuals.go:259` detects missing epics, while the
forecast-only guard is at `server/planning/actuals.go:445`;
`app/js/planui.js:1940` handles upload completion using mutable current-plan state.
The commands and observed outputs are in
`docs/audits/2026-09-05/README.md` and
`docs/audits/2026-09-05/observations.txt`. Findings and proposed corrections live in
`docs/MANAGER-AUDIT-2026-09-05.md`; this lesson does not define product requirements.
