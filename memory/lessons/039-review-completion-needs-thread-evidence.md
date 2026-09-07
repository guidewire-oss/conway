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

Attribute escaping in generated HTML is decoded by the HTML parser before
Bootstrap reads `dataset`. Escaping only quotation marks can instead corrupt
literal entity-like text. Compare the rendered tooltip with the original text
before accepting a double-escaping claim.
Provenance: observed 2026-09-06 through the actual Bootstrap tooltip assertion in
`tests/browser/bootstrap-adoption.mjs`, including ampersands, angle brackets and
a literal `&amp;`, with
`go test -race -count=1 ./server -ginkgo.focus='Bootstrap adoption' -ginkgo.no-color -ginkgo.succinct -timeout=3m`:
`ok conway/server 38.129s`. Existing escaping and the verification contract remain
canonical.

Strict browser diagnostics can expose omissions in the isolated host itself.
Supply intentional fixture resources rather than broadly ignoring console
errors: the acceptance shell requested a favicon absent from its static host.
Provenance: observed 2026-09-06 in the first console-error collection run
(`FAIL conway/server 15.360s`, only `/favicon.ico` returned 404). After the exact
favicon route returned 204,
`go test -race -count=1 ./server -ginkgo.focus='Bootstrap adoption' -ginkgo.no-color -ginkgo.succinct -timeout=3m`
returned `ok conway/server 16.284s`. See `tests/browser/bootstrap-adoption.mjs`.

Killing a browser test's Node process does not necessarily stop its browser.
Playwright launches Chromium in a detached process group on macOS and Linux.
Capture descendants before signalling the runner, allow graceful shutdown, and
join bounded cleanup before returning from the test command. Failure cleanup
must preserve parent links until that capture completes.
Provenance: observed 2026-09-06 with actual Chromium processes in
`server/browser_process_unix_test.go`. The Node-only cancellation reproduction
left four descendants; the shared helper's responsive deadline and blocked
event-loop cases removed all captured descendants.
`go test -race -v ./server -ginkgo.focus='browser process cleanup' -ginkgo.no-color -ginkgo.v -count=1`
returned `2 Passed`, `0 Failed`, `ok conway/server 19.140s`.
