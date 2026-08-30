# 019: resetting a "temporary" inline style to '' destroys a rendered style — restore, don't clear

**Date:** 2026-08-30
**Provenance:** observed 2026-08-30 in-browser on the the reference plan (user report: "the timeline shrinks when I double-click the Atlas bar"); DOM diff showed the lead bar's inline style going from `left: 27.66%; width: 47.87%` to `left: 27.66%` after a click; cause `release()`/`pointercancel()` in app/js/drag.js setting `bar.style.width = ''`; fixed in PR #55.

drag.js's preview writes px widths over the bar's rendered `%` width, and the cleanup cleared with `= ''` instead of restoring. An absolutely positioned element with no width shrink-wraps to its content — every mere click collapsed the bar to its label width (577px -> 159px). Real drags masked it because the PATCH re-render rebuilds the DOM with fresh styles.

Rules:
- When a gesture borrows an element's inline style for a preview, snapshot the value at gesture start and restore THAT — never assign `''`, because the "temporary" value may be sitting on top of a rendered one.
- The masking pattern is the tell: a bug invisible in the primary flow (drag) but visible in the adjacent flow (click) usually means the cleanup runs in a path the primary flow's re-render erases.
- DOM-state assertions (compare `getAttribute('style')` before/after) catch this class; visual-only checks don't, because the wrong state still looks plausible.
