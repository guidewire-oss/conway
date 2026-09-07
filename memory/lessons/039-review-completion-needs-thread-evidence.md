# Review completion needs current-head evidence

A successful automated review check can still create unresolved comments.
An empty thread list while that check is running is only provisional. After
the latest commit's review completes, inspect unresolved threads and pagination
before reporting review completion.

Review claims also need comparison with the current source. This review round
reported missing behavior that was already present: navigation updates
`aria-current`, simulator metrics render cards, and the divergence test retains
empty team strings. Apply the existing verification contract rather than adding
duplicate code to satisfy a stale or mistaken description.

Provenance: observed 2026-09-06 through GitHub PR 81 review checks and thread
queries for commits `7509702` and `74ba7cc`. Current-source checks included
`app/js/main.js:223`, `app/js/simulator.js:311`, and
`server/planning/review_test.go:106`. The existing divergence regression ran
with `go test -v ./server/planning -ginkgo.focus='attributes aggregate divergence without generic team duplicates' -ginkgo.no-color -ginkgo.v -count=1`:
`3 Passed`, `0 Failed`, `ok conway/server/planning 0.463s`.

The verification contract in `docs/FACTORY_RULES.md` remains canonical.

Browser focus checks must exercise keyboard input. After pointer interaction,
programmatic `focus()` need not activate `:focus-visible`; Tab navigation does.
Keep text contrast checks separate from assertions about the focus indicator.
Provenance: observed 2026-09-06 while running the Bootstrap browser regression;
the initial programmatic-focus shadow assertion failed, and keyboard navigation
passed with `go test -race -count=1 ./server -ginkgo.focus='Bootstrap adoption' -ginkgo.no-color -ginkgo.succinct -timeout=3m`:
`ok conway/server 14.411s`. See `tests/browser/bootstrap-adoption.mjs`.
