# Derived context needs one authority

A queue label copied the scheduler's default instead of calling its existing
accessor, and reported engine ordering while the scheduler used stated priority.
Persisted event JSON likewise carried a zero sequence alongside the authoritative
SQL sequence. Defaults and audit projections can drift even when the main
calculation is correct; reuse their canonical accessor or storage field.

Provenance: observed 2026-09-06 during PR 81 review and independent regression
checks. `server/planning/schedule.go:183` defines the effective ordering;
`server/planning/readyqueue.go:170` now uses it. `server/db/readyqueue.go:68`
keeps the SQL event sequence authoritative and omits it from stored event JSON.
The regressions live in `server/planning/readyqueue_test.go` (effective ordering)
and `server/readyqueue_integration_test.go` (raw payload and caller sequence).
Re-ran on 2026-09-06 with isolated PostgreSQL for the integration command:

- `go test -race -count=1 ./server/planning -ginkgo.focus='ready-work queue' -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.succinct`:
  `ok conway/server/planning 1.382s`.
- `go test -race -count=1 ./server -ginkgo.focus='team ready-work queue persistence' -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.succinct -timeout=5m`:
  `ok conway/server 2.274s`.

Specification 025 Decisions 1 and 4 remain canonical for product behavior.
