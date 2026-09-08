# Cleanup failures must retain the original cause

A finally block guarantees entry, not completion of every cleanup step. An
assertion or rejected restore request can skip later cleanup and replace the
original journey failure. Attempt cleanup steps independently and preserve all
errors, with the original failure first when several errors must be reported.

Canon: specs/028-portfolio-forecasts.md:259. Provenance: observed 2026-09-07
via `node --test tests/prediction-cleanup.test.mjs tests/prediction-history.test.mjs`:
`4 passed, 0 failed`. Injected journey, restore and browser failures demonstrate
that every cleanup runs and the original error survives in the aggregate.
