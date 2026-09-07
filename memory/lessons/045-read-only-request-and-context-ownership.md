# Preserve read-only status and independent context loading

When adding a read-only POST workflow, inspect the shared request wrapper as well
as the endpoint. A wrapper may classify every POST as a save and announce success
or failure outside the feature's own request ownership checks.

Context catalogs and answers also have different lifetimes. Scope edits should
invalidate an answer without stranding catalog loading. An explicit empty filter
means all records; it must not be replaced by the original route filter during a
refresh. Keep these requirements in the feature specification rather than
duplicating implementation rules here.

Provenance: observed 2026-09-07 during source review of `app/js/planui.js`'s `req`
and `app/js/planning-assistant.js`'s `load`, then exercised by
`go test -race -v -count=1 ./server '-ginkgo.focus=planning assistant|assistant comparison|assistant question' -ginkgo.no-color -timeout=3m`
with the isolated PostgreSQL and browser test environment:
`SUCCESS! -- 22 Passed | 0 Failed | 0 Pending | 196 Skipped`.
Canonical decisions: specifications 027, section 11, Decisions 2 and 5.
