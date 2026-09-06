# Completion order must advance after every saved review

A timestamp and random ID form a stable sort but do not establish completion
order. Two reviews completed in the same second can sort in the opposite order,
leaving a previous-review guard unchanged after a successful save.

Use the database-assigned completion order for history and stale-context checks,
with the plan lock coordinating completion and relevant writers. Keep timestamps
as human-readable provenance. Canonical behavior and rationale remain in
specs/024-weekly-execution-review.md:311.

Provenance: observed 2026-09-06 through the independent concurrency review and
the deterministic equal-timestamp regression in
server/weeklyreviews_integration_test.go:273. The isolated PostgreSQL suite
`go test -race -count=1 ./server -ginkgo.label-filter=database -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.succinct -timeout 5m`
returned `ok conway/server 5.566s` after the completion-order change.
