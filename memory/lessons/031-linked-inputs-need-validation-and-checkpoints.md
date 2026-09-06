# Linked inputs need validation and checkpoints

A permissive upload parser is not sufficient protection for unattended updates.
Validate populated fields and canonical header aliases before conversion, and
check that accepted numeric forms retain their meaning after conversion. An
accepted row whose assigned work disappears can look like a successful update.

Source: `server/sheets/parse_test.go:50` rejects conflicting roster aliases;
`server/sheets/parse_test.go:72` rejects malformed populated planning metadata.
Observed 2026-09-06 via `go test -race ./...` with isolated PostgreSQL enabled:
`ok conway/server/sheets 1.584s` and `ok conway/server 13.025s`.

A due-source list is only a snapshot. Re-read pause status and due time after
acquiring the source lease, before fetching; an earlier source can delay the
batch long enough for a manager to pause a later one.

Source: `server/linksheets_integration_test.go:272` exercises pause during another
source's blocked fetch. Requirements remain in
`specs/023-linked-google-sheets.md:258`; this lesson does not replace them.
