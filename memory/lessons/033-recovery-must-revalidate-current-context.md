# Revalidate recovered inputs without rewriting historical evidence

A captured planning table can fail because the roster is incomplete, even when
its cells are intrinsically usable. Explicit review should validate against the
current plan and protect that review with a fingerprint. Keep the original
capture and its diagnostics unchanged. Automatic application remains conservative.

Similarly, a snapshot containing only some bound epics cannot support a completion
percentage for the whole initiative. Retain observed counts and explain the gap;
withhold whole-scope forecasts and comparisons until evidence is complete.

Provenance: observed 2026-09-06 through the independent Ginkgo regressions in
`server/linksheets_integration_test.go` and
`server/planning/review_regressions_test.go`. The full isolated-database run,
`go test -race ./...`, returned `ok conway/server 12.462s`,
`ok conway/server/planning 5.916s` and `ok conway/server/sheets 1.641s`.
The decisions remain in `specs/023-linked-google-sheets.md:298` and
`specs/017-planning-and-execution-usability.md:193`.
