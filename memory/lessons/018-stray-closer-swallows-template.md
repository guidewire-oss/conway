# 018: a `<div>` opener with a stray `</span>` closer makes the browser swallow the rest of the template

**Date:** 2026-08-30
**Provenance:** observed 2026-08-30 in-browser on the the reference plan timeline (screenshot: buttons stretched 3801px tall, chart squeezed to a 563px right column); DOM walk showed `#tl-main` and `#tl-pod` as flex items inside the lens `.btn-group`; root cause `app/js/planui.js` timeline controls template; introduced by the `.seg` to `.btn-group` migration (PR #52).

The timeline controls template opened `<div class="btn-group">` and closed with `</span>`. HTML parsers IGNORE a stray `</span>` with no open span — so the first `.btn-group` (Bootstrap: `display: flex`, row) never closed and swallowed `#tl-main`/`#tl-pod`. Result: buttons became full-height flex columns, the chart squeezed into the leftover width. No test caught it because the markup was an untestable inline template.

Rules:
- When migrating tag types (span → div), grep the template for BOTH the opener and its closer — a mismatched closer is ignored silently, never an error.
- Keep shared markup in exported pure string functions (like `timelineControlsHTML`) with a tag-balance test; inline `innerHTML = \`...\`` templates in render closures cannot be tested.
- Symptom signature for this bug class: a flex container that is far taller than its content, or a sibling element appearing INSIDE an unrelated container in the DOM walk.
