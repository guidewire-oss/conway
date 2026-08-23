# Conway UI component registry

The design system is **Bootstrap 5.3** (vendored, MIT) with Conway's identity
layer in `app/css/conway.css` (palette on `--bs-*` variables, Public Sans /
IBM Plex Mono, radii, legacy token aliases). This registry is the contract
for how UI work happens:

> **Adopt Bootstrap components wholesale. Extend with a `cv-` component only
> when Bootstrap lacks one — and build the extension from Bootstrap
> utilities and variables, never beside them. Domain visualizations draw
> from the token layer only.**

## Adopted (use Bootstrap's markup/classes directly)

| Need | Use | Notes |
|---|---|---|
| Menus | `dropdown` + `dropdown-menu` + `dropdown-item` | see the nav in `app/index.html` |
| Modals | `Modal` JS via `app/js/modal.js` (`openModal`/`closeModal`) | adapter, see extensions |
| Buttons | `.btn .btn-primary` / `.btn-secondary` | bare `<button>` matches `.btn-secondary` via tokens |
| Inputs | `.form-control` / `.form-select` | bare inputs match via tokens; checkbox/radio/range stay native |
| Badges/pills | `.badge` | |
| Alerts/callouts | `.alert` | |
| Progress | `.progress` / `.progress-bar` | |

## Extensions (`cv-` components)

These exist because Bootstrap has no equivalent. Each is built from
Bootstrap utilities + variables; none re-invents a framework primitive.

### `modal.js` — the modal controller

Bootstrap's Modal, adapted onto Conway's legacy overlay shape (an overlay
div + innerHTML box + `hidden` toggling) so ~10 call sites migrated without
template rewrites. Wraps the box in `modal-dialog`/`modal-content` (BS
requires the structure), provides `openModal`/`closeModal`, and adds two
deliberate deviations from BS defaults:

- **No click-outside-to-close** (`backdrop: 'static'`): an in-progress form
  should not vanish on a stray click. The ✕ is the exit.
- **ESC closes regardless of focus**: BS wires ESC through its focus trap;
  our overlays don't reliably hold focus, so a document-level handler does
  it (added on `shown`, removed on `hidden` — never stacked).

Also: one modal at a time (opening a second hides the first), and the legacy
`hidden` attribute stays callers' source of truth, synced on every hide path.

### Inspector lists — `.insp-list` (both node panels)

The pod inspector's initiative/edge rows: numbered, row-ruled lists with a
hanging grid so wrapped names align under the name (not under the number).
Bootstrap has no "node inspector"; this composes `list-group`-style rows
from tokens. See `#netpanel`/`#plan-netpanel` in `app/css/style.css`.

## Domain visualizations (token layer only)

Never Bootstrap components — they are Conway's product. They draw from
`--bs-*` variables (via the legacy aliases) so any future theme change
re-skins them for free. One known exception: the network graphs' heat
gradient (`heatColor()` in `app/js/netgraph.js`) interpolates between the
status colors in code — d3 needs concrete values, not CSS variables; if the
palette moves, that interpolation moves with it.

- Timeline (`app/js/timeline.js`): bars, buffer tails, target diamonds, bands
- Order table + heatmap (`app/js/order.js`)
- Network graphs (`app/js/netgraph.js`, d3)
- Pod lens / pod sheet
- Fever chart, tornado, CDF (Observe)

## Rules for new UI work

1. Need a generic control? Bootstrap's class, straight from its docs.
2. Bootstrap lacks it? `cv-` component in this registry, built from
   utilities + variables, with the reason recorded here.
3. A domain visualization? Tokens only — no hardcoded colors, radii, or
   shadows; everything resolves through `--bs-*`.
4. The legacy token aliases (`--panel`, `--accent`, ...) resolve into the
   system; new code should prefer the `--bs-*` spelling.
