# Progressive discovery tracks actual presentation

Changing a catalog dialog into a paged introduction changes acknowledgement
semantics: only the visible update has been presented. Preserve independent
destination-visit state and cover both Bootstrap and the supported modal fallback.
The fallback exposes visibility through `hidden`, without Bootstrap's `show`
class. A check of that class alone misses subsequent page acknowledgements.

Canon: specs/022-feature-announcements.md:201. Provenance: observed 2026-09-07
via the six-suite Chrome acceptance run (`go test -race -v -count=1 ./server
-ginkgo.label-filter=browser -ginkgo.no-color -timeout=6m`, output: 6 Passed,
0 Failed), including the new fallback paging journey in
tests/browser/announcement-recovery.mjs.
