# Select forecast representatives before scoring outcomes

Shared scope can be transitive: a later prediction may connect two earlier
records that share no keys directly. Build complete overlap groups before
selecting the earliest representative. Keep that representative when its outcome
is pending or excluded; replacing it with a later completed result introduces
outcome-based selection. Distinct work still need not be statistically independent.

Canon: specs/029-forecast-history-validation.md:123. Provenance: observed
2026-09-07 via `go test -race -v -count=1 ./server/planning
-ginkgo.focus='forecast history validation' -ginkgo.no-color`:
`SUCCESS! -- 7 Passed | 0 Failed`. Cases include a later bridging record, shared
children under different epics, and exact mixed-cohort coverage denominators.
