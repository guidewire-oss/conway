# Explicit form sizing must survive legacy adoption

A control can declare Bootstrap's default size and still become compact when a
global MutationObserver adds a size class later. Inspect both stylesheet
overrides and dynamic form adoption when peer inputs and buttons differ.

Preserve explicitly authored framework sizing. Keep legacy fallbacks limited
to controls without an explicit component class. Exercise geometry after the
observer has rendered dynamic content, including both themes.

Provenance: observed 2026-09-07 by reading `app/js/forms.js` during the plan
control audit; its search branch unconditionally added `form-control-sm`.
The decision is recorded in
`specs/033-consistent-plan-controls-and-samples.md:99`; rendered peer geometry
is covered by `tests/browser/bootstrap-adoption.mjs`.
