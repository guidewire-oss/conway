# Component adoption rules to prevent Bootstrap debt

## Lesson

The planning-page audit (2026-08-26) found 47 hand-rolled input overrides, a
bespoke tooltip system, and custom segmented controls — all duplicating
Bootstrap components that were already vendored. Root cause: the design system
predates Bootstrap's adoption, and nothing forced new UI through BS components.

Rules going forward (spec 011):

1. **Forms**: every input/select/textarea gets `form-control`/`form-select`
   via `forms.js` (delegated injector + MutationObserver). NEVER restyle
   background/border on a form element with per-component CSS — change the
   `--bs-form-control-*` bridge vars in conway.css instead.
2. **Tooltips**: one system. Prefer the delegated glossary tip (`data-tip`)
   for term definitions; do not add a second custom tooltip div, and do not
   mix native `title` with it in the same view.
3. **New interactive groups** (segmented controls, toolbars): use BS
   `btn-group`/`btn` with the theme bridge, not bespoke `.seg`-style classes.
4. **Naming**: never introduce a class that shadows a BS name (`card`, `alert`)
   — it double-styles or silently blocks migration.
5. When adding a component that BS ships, either adopt BS or write a comment
   in the spec explaining why not.

## Provenance

- Audit 2026-08-26 (spec 011): grep counts — 47 input overrides, 0 BS
  utilities in views, `.card` shadowing BS's.
- PR #44 incident: hand-rolled tooltips made week-date hover feel broken.
