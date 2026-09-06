# Embedded guides need navigation boundaries

A reusable documentation iframe must distinguish its local document from an
external page. A cached loaded flag alone cannot establish that its document is
still accessible. Open publisher references separately and recheck the frame
before trying to reuse it.

Also handle Escape from the innermost reader interaction first: dismiss search
or mobile contents before allowing the containing guide to close. Otherwise a
reader trying to dismiss a result list loses the whole help context.

Authenticated browser testing also showed that assigning a fragment while an
iframe's modal is hidden can leave the reader at the introduction. Reassigning
the same fragment on reopening does not restore the target either. Explicitly
position the requested heading after frame load and modal presentation. Check
the heading's visible position, not just the URL or the separate-tab link.

Provenance: observed 2026-09-05 through local browser sign-in and contextual
Home Help: the tutorial heading remained more than 5,000 pixels below the
viewport before correction, then appeared at approximately 145 pixels afterward.
Repeated Scoreboard Help and Data Quality Help also landed at their headings.
The lifecycle regression is exercised by `tests/shell-ux.test.mjs`.

Provenance: observed 2026-09-05 during the documentation interaction review and
the `tests/shell-ux.test.mjs` iframe lifecycle regression check. Implementation
references: `app/js/docs.js:13`, `app/js/docs.js:47` and
`app/js/manual-reader.js:64`. The design decision is recorded in
`specs/012-in-app-usage-guide.md:221`.
