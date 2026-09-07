# Recheck ownership after consuming response bodies

Receiving response headers and consuming the error body are separate async
boundaries. A modal can close, reopen, or render newer controls between them.
Check the current render after the body resolves before displaying its error.
The canonical behavior is recorded in specs/026-reliable-evidence-foundation.md:159.

A race fixture must also establish which render it observes. Capturing a table
handle before a Playwright click lets an earlier request detach that table while
the click waits for actionability. Capture it synchronously during click dispatch
and wait for that exact table to be replaced before starting the next mutation.

Provenance: observed 2026-09-07 while addressing PR 82. Temporary diagnostics
showed a mutation from render ticket 3 returning while ticket 4 was active.
The corrected fixture reached all eight action/boundary cases. Running
`go test -race -v -count=1 ./server -ginkgo.focus='evidence sources browser'
-ginkgo.no-color -ginkgo.v -timeout=2m` passed with `ok conway/server 10.230s`.
The same test against the previous controller at a27ec69 failed with
`roster late error must not paint after reopen`; the old handler displayed
`Delayed snapshot mutation error` in the reopened modal. The current controller
was restored afterward.
