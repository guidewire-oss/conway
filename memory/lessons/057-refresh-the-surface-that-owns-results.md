# Refresh the surface that owns an asynchronous result

When controls move into a body-mounted drawer, their result callbacks must
update that drawer. Updating the original page can leave a working request
appearing inert. Keep editable drafts outside the result region so repainting
the answer does not erase user input.

Provenance: observed 2026-09-08 while tracing the baseline Compare handlers in
`app/js/planui.js` at commit `4ad5285`: both stored their result then called
`renderCurrentPlanView`, while `.bl-drawer-overlay` lived outside that view.
The accepted ownership decision is
`specs/015-baselines-drawer.md:208`; browser coverage lives in
`tests/browser/baseline-comparison.mjs`.
