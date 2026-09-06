# Theme consistency is not component adoption

A Bootstrap variable bridge can make custom controls resemble the framework
while still duplicating its buttons, forms, cards and layouts. Inspect template
classes and base CSS together; color consistency alone is insufficient evidence
of adoption. Keep application selector hooks without retaining duplicate visuals.

The durable delivery rule is in `AGENTS.md`; the component registry and
specification remain canonical rather than this lesson defining a second rule.

Provenance: observed 2026-09-06 via inspection of `app/css/style.css`,
`app/css/readyqueue.css`, `app/js/readyqueueui.js` and `app/js/forms.js`;
maintainer explicitly requested Bootstrap adoption before the next feature and
long-term retention. See `specs/011-bootstrap-adoption-debt.md` Decision 2 and
`docs/COMPONENTS.md`.

Bootstrap color utilities can override existing state colors with `!important`.
Its solid semantic badges also assume foregrounds based on the stock palette;
a custom theme requires rendered contrast checks, including alpha backgrounds
on body and card surfaces. The independent browser acceptance exposed a light
badge at 4.39:1 after class adoption. Keep that check with the component tests.

Provenance: observed 2026-09-06 via `server/bootstrap_browser_test.go` with
`go test ./server -ginkgo.focus='Bootstrap adoption' -count=1`, and the vendored
`.text-bg-secondary` rule in `app/vendor/bootstrap/bootstrap.min.css`.

Do not use a broad HTML-tag replacement regex as the sole migration audit for
JavaScript template strings. Nested templates can be swallowed by an earlier
opening tag, leaving whole groups of controls unchanged. Inspect each target
control and render the real workflow. The game lever actions exposed this gap.

Provenance: observed 2026-09-06 via PR 81 review and targeted inspection of
`app/js/gameui.js` after commit `38ca822`; the independent review found 13
buttons missing Bootstrap classes inside nested game templates.
