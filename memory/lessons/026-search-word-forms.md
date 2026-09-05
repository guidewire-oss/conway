# Search shorthand is not word-form matching

Subsequence search still misses `rotate` against `rotation`: the final `e`
does not exist after the matching letters. Keep identity unchanged and apply
word-ending convenience only in the shared display filter, so bars, held work,
outside-view notices and match counts agree.

Provenance: observed 2026-09-05 with the pre-change matcher returning
`{"rotate":false,"rotation":true}` for a generic credential-rotation name.
The decision is `specs/010-timeline-lens-filters.md:164`; regressions are in
`tests/timeline-search.test.mjs`. The browser replay found one initiative across
all eight assigned teams after the correction.
