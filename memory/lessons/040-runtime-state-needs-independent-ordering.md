# Runtime state needs independent concurrency guards

A configuration version does not serialize background execution. A settings
request can read a source before a capture completes, then save after completion
without encountering a version conflict. Preserve completion-derived scheduling
state atomically in the update unless the user changes cadence or enablement.

Second-resolution timestamps also cannot order captures that finish in the same
second. Use a durable insertion sequence for attempt history; keep displayed
capture times as observations rather than identifiers.

Canonical decisions: [specification 026](../../specs/026-reliable-evidence-foundation.md#11-decision-record).
Implementation: `server/db/evidence.go` (`UpdateEvidenceSource`, `EvidenceRuns`)
and `server/db/migrations/0026_evidence_sources.sql`.

Provenance: observed 2026-09-06 during independent capture review and regression
acceptance. `go test -race ./...` returned `ok conway/server 10.714s`,
`ok conway/server/evidence 1.904s`, and `ok conway/server/jira 2.285s`.
With the isolated database and browser opt-ins, `go test -race -v -count=1
./server -ginkgo.label-filter=browser -ginkgo.fail-on-empty -ginkgo.fail-fast
-ginkgo.no-color -ginkgo.succinct -timeout=6m` returned
`SUCCESS! 1m36.723701375s` and `ok conway/server 98.422s` for all five browser cases.
