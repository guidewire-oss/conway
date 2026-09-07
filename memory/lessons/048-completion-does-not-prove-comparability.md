# Completion does not prove forecast comparability

A captured initiative can have a known finish while its work includes teams
outside the recorded schedule. Reuse the execution evidence's completeness
semantics, but check comparable scope independently before counting outcomes.
Otherwise the same unplanned scope can be excluded while pending and enter
coverage merely because its remaining issues become done.

Canon: specs/028-portfolio-forecasts.md:214. Provenance: observed 2026-09-07
via the failing Ginkgo case `withholds unplanned team work even when all captured
children finish`, followed by an unconditional unplanned-team exclusion in
server/planning/prediction.go. The corrected assessment cases passed in
`go test -race -v -count=1 ./server ./server/planning
-ginkgo.focus='portfolio forecast API|planning assistant browser|prospective forecast assessment'
-ginkgo.no-color -timeout=3m` (planning: 5 Passed, 0 Failed). Subsequent full
Go race checks also passed after adding mixed-cohort denominator coverage.
