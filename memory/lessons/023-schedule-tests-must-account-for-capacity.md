# Schedule tests must account for capacity

A plausible start date or lane-count attribute does not establish a feasible
schedule. Check delivered effort against the selected lanes and capacity loss,
then check every occupied week against physical capacity. Exercise the first
placement as well as later placements: a lazily created calendar can make the
first item bypass a policy that rejects every following item.

At the presentation boundary, distinguish input assignments from placed slices.
An absent slice can mean rejected work, and a lane phase needs its own time
interval. A text assertion alone cannot establish that the chart draws it.

Provenance: observed 2026-09-05 via read-only recomputation of a 29-by-35 reference
plan and per-week capacity accounting; decisions and regression expectations are
recorded in specs/018-scheduling-capacity-and-timeline-correctness.md:36 and :108.
The spec remains the source of truth for policy.
