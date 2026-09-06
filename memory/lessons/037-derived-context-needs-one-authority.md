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
The independent ordering and raw-payload tests ran with the queue/review suite:
`ok conway/server/planning 0.504s` and `ok conway/server 1.335s`.
Specification 025 Decisions 1 and 4 remain canonical for product behavior.
