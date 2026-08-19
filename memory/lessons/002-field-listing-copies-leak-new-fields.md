# Lesson: a copy that lists fields silently drops every field added later

## Date
2026-08-19

## Context
Spec 001 extends `planning.Initiative` with eleven optional sequencing
attributes — stated priority, target date, locks, tier, cost of delay, earliest
start, kit percentage, carryover. The scheduler exists to test those dates.

`applyLevers` in `server/planning/simulate.go` cloned each initiative like this:

```go
i2 = append(i2, Initiative{Name: it.Name, Description: it.Description,
    Leads: it.Leads, Work: w})
```

That was correct when `Initiative` had four fields. The moment the struct grew,
`POST /api/plan/{id}/schedule` with any lever attached would compute an order
from initiatives whose target dates, priorities and locks had all been reset to
zero — and every verdict would read "no-date". No test failed, nothing logged,
and the response still looked structurally valid.

## Root cause
A field-listing copy encodes the struct's shape at the time it was written, but
it does not fail when that shape changes — the compiler is satisfied by any
subset. The defect surfaces only in the one code path that combines the new
fields with the old copy, which is exactly the path least likely to be covered
when the fields are new.

## The fix
Copy the whole struct and replace only what must differ:

```go
clone := it
clone.Work = w   // the map is the one field that needs its own copy
i2 = append(i2, clone)
```

This is correct for every field added after today, including ones nobody has
thought of yet. A spec in `server/planning/schedule_test.go` now sets all eleven
attributes, runs `ApplyLevers`, restores the deliberately-copied map and asserts
the whole struct is unchanged — so the assertion covers new fields automatically
rather than needing one line per field.

## Where to look for the same shape
Any `X{A: x.A, B: x.B}` literal built from another `X`. In this repo the
copy-on-write clones in the lever and simulation paths are the population;
`docs/examples/hooks/field-coverage-check.sh` is the factory's example hook for
exactly this class of defect, and arming an adapted version of it for
`Initiative` is an open option rather than something this change did.

## Provenance
Observed 2026-08-19 while implementing spec 001 slices 1-2. The field-listing
copy was at `server/planning/simulate.go` in `applyLevers` before commit
`743b0da`; the `Initiative` fields it dropped are defined in
`server/planning/planning.go` and specified in
`specs/001-plan-execution-order.md` §7. Confirmed by running
`go test ./server/planning/` against the new spec with the old copy restored:
the assertion fails on the dropped attributes.
