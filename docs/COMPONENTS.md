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
| Buttons | `.btn .btn-primary` / `.btn-secondary` | Explicit classes in templates; no bare-button visual fallback |
| Inputs | `.form-control` / `.form-select` | Explicit template classes; `forms.js` retains legacy/dynamic form adoption |
| Checks and ranges | `.form-check-input` / `.form-range` | Keep native input semantics and accessible labels |
| Cards | `.card` with `.card-body` or padding utilities | `.panel-card` may remain as an application hook |
| Choice groups | `.btn-group` with named `role="group"` | Existing state owner maintains `.active` and `aria-pressed`; do not add a competing toggle controller |
| Badges/pills | `.badge` with an appropriate color utility | State remains understandable from text |
| Alerts/callouts | `.alert` | |
| Contextual help | Named `.btn` with the delegated Bootstrap tooltip | `term(id)` for glossary entries; `helpButton(text, label)` from `terms.js` for other explanations. The label supplies the accessible name. Table help must not trigger sorting. |
| Progress | `.progress` / `.progress-bar` | |
| Layout and spacing | Grid, flex, gap, padding and margin utilities | Use responsive variants; keep only necessary domain sizing rules |
| Evidence tables | `.table` inside `.table-responsive` | Wide data scrolls within its container |

Compact selectors embedded in labels or calendar rows use `w-auto` and the
appropriate inline/flex utility. The documentation reader keeps its editorial
cell padding through scoped `.table` rules; compact data-table density is not
appropriate for paragraph-length reference material. See specification 011,
Decision 3, for the context sizing and accessibility acceptance contract.

A containing card owns the surface and padding for its alternate states.
Waiting and closed-state explanations within it should not add a second card
unless they represent a separate, meaningful group of information.

Neutral metadata badges use `bg-body-secondary text-body` so both surface and
text follow the active theme. Compact control groups wrap within their container;
delete actions must preserve a usable target. Shared help keeps a native `title`
fallback alongside the delegated tooltip. Test actual visibility when a state
owner toggles `hidden` on framework components.

KPI collections use responsive framework layout rather than forcing every metric
onto one row. Table-row actions use `btn-sm`; inline identifier fields use
intrinsic width. Base table overrides must not erase contextual table colors.
Form focus borders and rings use the shared primary and focus-ring tokens.
Plan setup settings wrap responsively; date inputs and actions embedded in
account tables use compact framework sizing. Plan period and capacity-loss
fields retain a bounded 5rem width for their short numeric ranges; Bootstrap
has no intrinsic numeric-width utility, while its flex utilities handle wrapping.
Game timing fields reserve room for the largest allowed value and native
spinners: 4rem for single-digit settings, 6rem for the timer. Compact timeline
exception and simulator row actions retain shared Bootstrap button styling.
What-if lever selectors use intrinsic widths with a wrapping target group.
Link RGB aliases derive from the primary token, including opacity utilities.
Navigation colors belong in Bootstrap dropdown variables.
Timeline inspector spacing remains domain geometry, including its mobile padding,
so a fixed padding utility must not override that responsive rule.

Keep semantic token families consistent: base, emphasis, subtle background and
border colors must describe the same theme tone. Neutral badges use body surface
and text utilities throughout the app. Glossary and contextual help share escaped
button markup; static help retains a native title too. Bootstrap margin
utilities own the help-button gap, without additional literal separator spaces.
A domain control with its own focus outline suppresses the framework shadow so it has one indicator.

For a mutually exclusive preference such as the guidance persona, use native
radio inputs with Bootstrap `btn-check` and associated button-styled labels.
Keep the group named and preserve focus and the saved choice when content updates.

CI selects browser acceptance by the shared Ginkgo `browser` label, so suite
names can describe their own workflows without changing coverage. The Bootstrap
suite runs `tests/browser/bootstrap-adoption.mjs` against its isolated Go host.
Acceptance checks uncaught exceptions, console errors and failed requests;
request cancellations reported as `net::ERR_ABORTED` are excluded. The minimal host supplies
a no-content favicon response so an incidental browser request is not confused
with a product resource failure. Browser workloads share bounded process-tree
cleanup on macOS and Linux; cancellation acceptance covers responsive and
blocked runners with detached Chromium processes. The twelve-minute CI package
budget accommodates three three-minute workloads and their bounded cleanup.

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
  should not vanish on a stray click. The close button is the exit.
- **ESC closes regardless of focus**: BS wires ESC through its focus trap;
  our overlays don't reliably hold focus, so a document-level handler does
  it (added on `shown`, removed on `hidden` — never stacked).

Also: one modal at a time (opening a second hides the first), and the legacy
`hidden` attribute stays callers' source of truth, synced on every hide path.

### Native disclosures

A standalone `details`/`summary` is an intentional native HTML control. Keep
its built-in keyboard semantics. Use Bootstrap spacing/border utilities around
it; an accordion is appropriate only when that grouped interaction is required.

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
- Timeline scrolling and resizable labels (`app/js/timeline-viewport.js`): native
  horizontal overflow, a shared time canvas and sticky labels. The focusable
  separator supplies pointer and keyboard resizing; Bootstrap has no equivalent
  Gantt geometry or resizable label component. Standard actions use Bootstrap.
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

## Adoption review before feature completion

Plan action buttons and timeline search inputs use the default Bootstrap size.
Toolbar layouts align control bottoms with `align-items-end`, keep visible
labels above fields, and wrap using framework flex and column utilities. Do not
add ID-specific fonts, padding or heights to peer controls. The legacy form
initializer preserves explicitly declared `form-control` sizing; its compact
search fallback applies only to unstyled inputs. See
`specs/033-consistent-plan-controls-and-samples.md`, Decision 1.

Check the rendered UI, including content added after a request completes:

- Generic controls carry Bootstrap classes in their source templates.
- Custom CSS does not duplicate a component's base states, borders or spacing.
  Use Bootstrap component variables for brand changes.
- State selectors and application event handlers survive the migration; one
  owner controls each selection. CSS classes alone do not add ARIA semantics.
- Keyboard focus, disabled actions, labels, wrapping and table scrolling remain
  usable in light and dark themes and at a 360px viewport. Normal-size text on
  enabled controls and badges meets 4.5:1 contrast, including transparent
  backgrounds composited over the containing surface. See
  [WCAG contrast guidance](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html).
- Any new custom extension has a specific gap documented in this registry.

The migration decision and acceptance criteria are maintained in
[specification 011](../specs/011-bootstrap-adoption-debt.md). Existing custom
visualization geometry and editorial reading layouts need not be rewritten as
utilities just to increase class usage.

Framework references checked on 2026-09-06: [buttons](https://getbootstrap.com/docs/5.3/components/buttons/),
[button groups](https://getbootstrap.com/docs/5.3/components/button-group/),
[forms](https://getbootstrap.com/docs/5.3/forms/overview/) and
[responsive grid](https://getbootstrap.com/docs/5.3/layout/grid/).

Capture sources extend the existing Snapshots dialog with Bootstrap cards, grid
forms, validation, badges and responsive tables. Their settings retain drafts
after failures; status refresh never silently changes the selected snapshot.
Captured freshness is reused in Measure and Review execution. No custom control
CSS is needed for this workflow; source-scoped identity strings wrap in tables.

The integration CI job allows 30 minutes for the five-minute database gate,
12-minute browser gate, dependency installation and cleanup.

History validation extends the prediction detail's existing evidence selector
with a Bootstrap outline action. Results use Bootstrap alerts and responsive
grid cards; full-entry explanations are progressively disclosed with native
details/summary. No additional component CSS or duplicate source selector is used.

Portfolio forecasts reuse Bootstrap grid forms, cards, alerts and responsive
tables. Feature discovery uses a single-update Bootstrap modal with a native
select and previous/next buttons; menu dots retain a visually hidden label.
Forecasts introduce no custom CSS. Feature discovery retains the existing
`app/css/announcements.css` extension: compact dot geometry, legacy theme
variables, long-text wrapping and narrow-screen touch targets. Bootstrap owns
the controls, layout and modal lifecycle through the shared modal adapter.
The dot and legacy theme bridge remain the documented gaps; this increment
removes the previous custom list styling rather than adding component rules.

Prediction history extends Forecasts with native disclosure sections, Bootstrap
forms, list-group buttons, cards and responsive tables. Recording appears only
after a comparison. History and capture errors stay beside the affected action;
capture refresh preserves selection when available. No custom CSS is introduced.
