# Lesson: a "late initiative" test fixture must beat the scheduler, not just miss a date

## Date
2026-08-22

## Context
Writing Story 5 (remedies) specs, the first fixture was the obvious one: an
initiative with a target date the drum cannot reach, queued behind an undated
predecessor. The guard spec failed — the target came back **on-time**. Reason:
`ComputeSchedule` runs five dispatch rules and keeps the best by weighted
lateness, and `minimum-slack` had simply promoted the dated initiative ahead of
the undated one. The scheduler had already rescued the date the fixture was
supposed to make it miss.

The second attempt failed the same way for a subtler reason: a locked
predecessor at priority 1 cannot be displaced by raising the target's priority,
because locked ties break by sheet order (spec 001 §10 Q8) — so AC 5.2's
raise-priority remedy could never exist in that fixture.

## Root cause
A TDD fixture for "a date that will not fit" is a claim about what the engine
does with the inputs, and the engine is aggressively good at reordering. The
fixture has to be late *after* the scheduler has done its best: the miss must be
structural (locked contention, chain longer than the window), not something a
smarter order fixes.

## The fix
For a contention fixture: give the competitor its own tight date AND a priority
lock ahead of the target — then the rules legitimately save the competitor and
the target's miss is real. Verify the fixture with a guard spec (fail fast with
"the fixture must produce a late date") before writing behaviour specs against
it, and debug-print the actual schedule (`startWeek/commitWeek/verdict/binding`)
before asserting anything about remedies.

## Provenance
Observed 2026-08-22 in server/planning/remedies_test.go: the guard spec
"finds the fixture's miss to begin with" failed twice with `on-time` before the
Early/Late/Other fixture (Early unlocked+dated w14, Late locked p2+dated w20,
Delta 1 track) produced Late at 3 weeks late. The locked-tie behaviour is
`rankOrder` in server/planning/schedule.go (locked sorted by stated priority,
then sheet index) with spec 001 §10 Q8.
