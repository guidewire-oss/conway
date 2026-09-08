# Bound retained evidence before accepting a registration

A training-only limit can accept a model that can never be assessed: pending
and boundary predictions may be retained even though they were not fit samples.
Apply the assessment limit to the entire archive before accepting it. Likewise,
an immutable collection needs a creation limit or pagination before its list
limit is reached; serialize competing writes and recover existing retry keys
before checking capacity.

Canonical decisions: specs/031-prospective-forecast-registration.md:120.
Implementation: server/planning/registration.go:41 and
server/db/registrations.go:54. Provenance: observed 2026-09-07 during the
registration review; the model suite run with `go test -race -v
./server/planning -ginkgo.focus="forecast model evaluation" -ginkgo.no-color`
reported `9 Passed | 0 Failed`, including exactly 5000 versus 5001 retained
entries. The concurrent database test covers two keys competing for the final
registration slot.
