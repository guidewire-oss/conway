# Analytics provenance needs visible state

A source selector is also an identity label. Hiding it when only one source
exists conceals the provenance users need to interpret the data. Keep the source,
capture time and association visible; distinguish unavailable metadata from a
confirmed missing association.

Scenario inputs need their own origin label even when they reuse measured team
statistics. A task imported without a team must retain an explicit missing
selection; a browser select otherwise chooses its first option and invents an
assignment.

Provenance: observed 2026-09-05 by inspecting `app/js/main.js`'s previous
single-snapshot early return and reproducing `podOptions('')` against a one-team
roster. Decisions are recorded in `specs/021-measure-source-context.md` section
11; source-state regressions are in `tests/measure-context.test.mjs`.
