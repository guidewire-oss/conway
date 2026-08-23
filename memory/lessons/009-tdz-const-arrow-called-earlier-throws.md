# Lesson: a const arrow function called earlier in the same scope throws — the temporal dead zone is positional

## Date
2026-08-23

## Context
Carrying form edits across a re-render, I added `applyLiveAssumptions()` right
after `host.innerHTML = ...` in renderOrder, and declared the helper as a
`const` arrow forty lines further down in the same function body. Every Order
render threw `ReferenceError: Cannot access 'applyLiveAssumptions' before
initialization` — and because renderOrder is async and the throw happened after
the container was painted with the schedule, the visible symptom would have
been "the form's buttons don't work", not an obvious crash. Cubic's review
caught it before the push; I had verified the previous commit in the browser
but re-verified only the new interaction, not the base render, after the
follow-up edit.

## Root cause
`const`/`let` bindings exist only from their declaration onward (the temporal
dead zone). Function *declarations* hoist with their body; arrow functions
assigned to consts hoist only the binding. Calling site position — not
definition existence — decides whether it throws.

## The fix
Two rules:
1. Helpers called mid-function get `function` declarations, at any position.
2. **Re-verify the base path after any edit to a code path you already
   verified.** The browser check that mattered after this change was "does the
   Order view still render at all", not just "does the new carry feature work".

## Provenance
Observed 2026-08-23 on PR #19 round 3: cubic thread PRRT_kwDOTMR4Hs6bdiF8
("Every Order render throws before it wires the form"). Fixed in 3def536 by
hoisting the helpers to module scope as function declarations; in-browser
verification then showed rows=10 with the period start carried across add.
