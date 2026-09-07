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
long-term retention. See `specs/011-bootstrap-adoption-debt.md:159` Decision 2 and
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

Check theme overrides on a containing element as well as the root palette.
A locally reintroduced legacy alias can hide an ancestor's component variable;
an inherited alias does not necessarily create the literal cycle suggested by
a source-only review. The rendered card test now supplies an ancestor override
and checks the actual background in both themes.

Provenance: observed 2026-09-06 via `server/bootstrap_browser_test.go` and
`tests/browser/bootstrap-adoption.mjs` in
`go test -race -count=1 ./server -ginkgo.focus='Bootstrap adoption' -ginkgo.no-color -ginkgo.succinct -timeout=3m`:
`ok conway/server 20.501s`. This current suite name replaces the historical
shared title prefix; CI selects both browser suites by label. See the component
contract in `docs/COMPONENTS.md`.

An exclusive preference can use native radio inputs with Bootstrap button
labels. If changing the preference replaces those inputs, restore focus to the
new checked input so native arrow navigation can continue.
Provenance: observed 2026-09-06 in `app/js/guide.js` and the real guidance
keyboard/persistence journey in `tests/browser/measure-bootstrap.mjs`;
[W3C radio-group guidance](https://www.w3.org/WAI/ARIA/apg/patterns/radio/)
fetched 2026-09-06. The component registry remains canonical.

Base component overrides can erase contextual variants even when they only set
variables. Check neutral and semantic tables independently; generic focus rules
should match framework specificity so validation focus states retain priority.
Provenance: observed 2026-09-06 in `app/css/conway.css` and the vendored Bootstrap
table/form rules. The rendered table, account focus and responsive layout checks
in `tests/browser/measure-bootstrap.mjs` passed with
`go test -race -count=1 ./server -ginkgo.focus='Bootstrap adoption' -ginkgo.no-color -ginkgo.succinct -timeout=3m`:
`ok conway/server 16.367s`. See `specs/011-bootstrap-adoption-debt.md:183`, Decision 3.

Settle component transitions before comparing an existing control with a newly
inserted style reference. A fresh button starts in the new theme while an
existing button may still be interpolating from the old one.
Provenance: observed 2026-09-06 in the simulator delete-style browser check
(`FAIL conway/server 15.953s`). Awaiting the actual button animations before
comparison produced `ok conway/server 39.004s` with
`go test -race -count=1 ./server -ginkgo.focus='Bootstrap adoption' -ginkgo.no-color -ginkgo.succinct -timeout=3m`.
See `tests/browser/measure-bootstrap.mjs`; the component registry remains canonical.
