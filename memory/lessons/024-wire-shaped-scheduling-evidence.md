# Scheduling evidence must preserve wire semantics

A fixture that fills omitted fields can hide a production rendering defect.
An idle team-week omits its initiative list in JSON; interpreting the absent list
as incomplete evidence caused a flat Gantt bar to fill a calendar gap. Test the
actual wire shape as well as the intended domain state.

Compare chart occupancy with authoritative weekly allocation, and account for
productive effort separately from ramp time. A visually plausible chart and a
low lateness score alone do not establish complete delivery.

Provenance: observed 2026-09-05 by running
`node --test --test-name-pattern='calendar gaps' tests/timeline-correctness.test.mjs`
with omitted idle-week initiative keys: `week 1`, `1 !== 0`. The decision record
is `specs/019-scheduling-audit-and-gantt-integrity.md:177`; the independent
regression is in `tests/timeline-correctness.test.mjs`.
