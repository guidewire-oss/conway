# Map API shapes to the view's vocabulary at the wiring seam

## Lesson

The pairwise baseline endpoint (spec 005) returns `{from, to, comparison}`,
but the compare card's renderer reads `result.baseline` — the live compare's
shape. The first live check rendered "moved from the baseline → v2 Aurora
first": the from-name silently degraded to a generic label because the wiring
passed the API's shape straight through.

Rule: when a view renders two flavours of response, map the second flavour's
keys onto the first's at the single wiring point (`if (res.from && !res.baseline)
res.baseline = res.from`), and assert the human-facing string in the live
probe — the row count and deltas looked right, and only the summary sentence
exposed the mismatch.

## Provenance

- Observed 2026-08-25 via in-browser probe on the GWCP plan: first pairwise
  card read "17 initiatives have moved from the baseline → v2 Aurora first";
  after the mapping fix, "...from v1 first cut → v2 Aurora first". Fix in
  app/js/planui.js (compare-to wiring).
