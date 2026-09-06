# Document model boundaries from the executed path

Conway's user guide must distinguish conceptual inspiration from implementation.
The five books confirmed by the product owner are recorded in
`specs/012-in-app-usage-guide.md:127` and its second decision. Keep their credits
in the canonical guide; do not attribute Conway's numeric heuristics to them.

When explaining a calculation or action, trace its production callers. A helper
with a plausible name is insufficient evidence of user-visible behavior. During
the documentation review, site latency utilities suggested a scheduling delay,
but the scheduler's `handoffWeeks` returns zero. Similarly, the database's
baseline deletion alone did not describe the handler's subsequent activation of
the newest remaining agreement.

Provenance: observed 2026-09-05 by reading `server/planning/schedule.go:268` and
`server/planhandlers.go:1142`, then correcting the guide against those callers.
The maintained explanation belongs in `app/docs.html`; this lesson records the
review method rather than a second formula reference.
