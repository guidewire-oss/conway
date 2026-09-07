# Race tests must hold the response used by the target view

Refreshing capture status requests snapshots for both the Measure context and
the open snapshot picker. Holding only the first matching HTTP response can
exercise the context while leaving the picker's stale response uncontrolled.
A passing test then says nothing about the intended picker race.

The regression now holds both old responses, changes a real snapshot name,
allows the newer refresh to render, and only then releases the stale responses.
Separate Escape checks retain hidden DOM, notification count and selection while
real capture/save requests finish, then confirm persistence after reopening.

Provenance: observed 2026-09-07 during independent acceptance review for PR 82.
`go test -race -v ./server -ginkgo.focus='evidence sources browser'
-ginkgo.no-color -ginkgo.v -count=1 -timeout=5m` returned
`ok conway/server 8.183s` with the corrected fixture and controller. Running the
same browser journey against the previous `app/js/snapshotsui.js` from a9deee7
returned `FAIL! -- 0 Passed | 1 Failed` at the stale snapshot-name assertion;
the current controller was restored automatically afterward.
Canonical lifecycle decision: specs/026-reliable-evidence-foundation.md:159.
