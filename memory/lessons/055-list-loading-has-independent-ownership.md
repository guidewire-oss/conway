# List loading has independent ownership

A form edit should invalidate its submitted action, not a concurrent catalogue
read. Give the list its own request ticket and status; after successful writes,
start a newer list read so an old response cannot erase the newly saved item.
Reapply action busy state to buttons created by list rendering.

Canonical decision: specs/031-prospective-forecast-registration.md:159.
Provenance: PR 86 comment 3954053386, inspected 2026-09-07; the focused manager
browser and registration integration command in
`/private/tmp/conway-registration-review-tests.log` reported
`3 Passed | 0 Failed` on this date. The browser holds the initial list while
editing, and separately delivers a pre-save list after save recovery.
