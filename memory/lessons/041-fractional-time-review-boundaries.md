# Check the actual boundary before rounding time

A review claimed fractional current seconds made integer-second capture timestamps
stale early. The threshold is an integer timestamp plus integer hours times 3600.
For that boundary, `now >= boundary` and `floor(now) >= boundary` are equivalent.
Rounding adds no correction. Evaluate both sides immediately before, at, and after
the boundary before accepting a proposed numerical fix.

Provenance: observed 2026-09-06 while resolving PR 82 review comments. A Node
`--input-type=module` check imported `evidenceFreshness` and
`captureFreshnessHTML`, exercised 35 combinations of 1/6/24/168/720 hours and
fractional offsets around the threshold, and returned `35 fractional-second
boundary cases passed for both freshness functions; flooring does not change the
result.` An independent reviewer also checked 21 cases with the same result.
Canonical freshness policy: specs/026-reliable-evidence-foundation.md:42.
