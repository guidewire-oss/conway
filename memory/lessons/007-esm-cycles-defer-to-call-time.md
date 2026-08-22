# Lesson: an ESM import cycle is safe only while both sides defer the other's exports to call time

## Date
2026-08-22

## Context
The Order view (app/js/order.js, where `esc` lives) needed the remedies
expander from app/js/remedyui.js — which imports `esc` from order.js. A cycle.
Node ESM resolves it: order.js starts evaluating, hits the remedyui import,
remedyui imports the in-progress order.js namespace, defines only functions
(nothing of order.js is called during remedyui's evaluation), and finishes;
order.js then continues. All 129 node tests pass with the cycle in place.

## Root cause / rule
A cycle breaks the moment either module *uses* the other's export while it is
still evaluating — e.g. a module-level `const TABLE = otherModule.f()` or a
default-parameter evaluated at definition time. Then the binding is in the
temporal dead zone and the import throws `ReferenceError: Cannot access
'X' before initialization` — not at the import site, but wherever the cycle
closes, which is a confusing stack trace away from the cause.

## The fix
Allowed the cycle because both usages are inside functions invoked after both
modules finish evaluating, and documented that precondition in a comment at
the import. The alternative — duplicating `esc` into remedyui.js — was
rejected on security grounds: two escaping implementations drift, and the
drift is exactly where XSS lives. If a third view module ever needs `esc`,
the right move is hoisting it to a shared util.js everyone imports, breaking
the cycle without duplication.

## Provenance
Observed 2026-08-22 wiring the remedies expander: order.js imports
optionsExpanderHTML from remedyui.js (app/js/order.js, the import sits above
`esc` with a comment), remedyui.js imports esc from order.js. Verified by
`node --test tests/*.test.mjs` printing `# pass 129  # fail 0`, and by
`node --check` on all three modules.
