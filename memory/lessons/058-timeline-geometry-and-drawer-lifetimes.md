# Keep visual geometry and request ownership tied to their actual context

Long Gantt spans need a shared time canvas: dates, overlays, occupied tracks and
idle tracks must use identical widths. A zero-duration scheduled slice needs a
checkpoint representation; filtering every zero-width bar hides that evidence.
PNG conversion must retain checkpoint positioning when replacing buttons with
plain labels. See specs/034-readable-scrollable-timelines.md:88 and
app/js/timeline.js:403.

A comparison result depends on both its input revision and its drawer session.
Invalidating pending responses alone leaves completed live deltas stale. A
stable drawer overlay survives content refreshes; a captured child region does
not. See specs/015-baselines-drawer.md:231 and app/js/planui.js:1082.

Provenance: observed 2026-09-10 via the isolated Ginkgo/Playwright Bootstrap and
linked-features browser workloads, including 154-week geometry, checkpoint
export, working-order invalidation and same-drawer refresh regressions.
