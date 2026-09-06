# Embedded guides need navigation boundaries

A reusable documentation iframe must distinguish its local document from an
external page. A cached loaded flag alone cannot establish that its document is
still accessible. Open publisher references separately and recheck the frame
before trying to reuse it.

Also handle Escape from the innermost reader interaction first: dismiss search
or mobile contents before allowing the containing guide to close. Otherwise a
reader trying to dismiss a result list loses the whole help context.

Provenance: observed 2026-09-05 during the documentation interaction review and
the `tests/shell-ux.test.mjs` iframe lifecycle regression check. Implementation
references: `app/js/docs.js:38`, `app/js/docs.js:61` and
`app/js/manual-reader.js:64`. The design decision is recorded in
`specs/012-in-app-usage-guide.md:221`.
