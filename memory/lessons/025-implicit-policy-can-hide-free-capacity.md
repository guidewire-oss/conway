# Implicit policy can hide free capacity

A missing forecast date does not necessarily mean a missing input date. An
implicit release gate can suppress otherwise legal placements and leave free
team tracks. Trace the binding cause before asking planners for more data.

When changing assumed limits to advisory warnings, preserve deliberately saved
constraints and expose the enforcement choice. Warn from final overlapping
occupancy so earlier releases receive the same evidence as later ones.

Provenance: observed 2026-09-05 using a generic three-team undated-work probe:
the third assignment had zero slices with constraint `lead`, although its team
had a free track. The regression is in
`server/planning/undated_scheduling_test.go`; the policy decision is in
`specs/020-undated-capacity-scheduling.md` section 11.
