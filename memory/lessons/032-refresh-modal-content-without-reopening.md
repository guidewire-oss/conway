# Refresh modal content without reopening it

Replacing content in an already visible Bootstrap dialog does not produce a
second `shown.bs.modal` event. Record presentation of recovered content directly,
and keep the existing modal open. Calling `openModal` again can replace the
original external focus-return target with the retry button inside the dialog.

Provenance: observed 2026-09-06 in
`tests/browser/announcement-recovery.mjs:33` (presentation acknowledgement) and
`tests/browser/announcement-recovery.mjs:39` (Escape restores the external trigger).
The final isolated browser run, `go test ./server -ginkgo.focus='linked features browser'`,
reported `ok conway/server 3.187s` with one passing acceptance spec.
The presentation and focus contract remains in
`specs/022-feature-announcements.md:180` and `specs/022-feature-announcements.md:192`.
