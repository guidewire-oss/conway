# Reproducing the manager audit

These diagnostic probes accompany `docs/MANAGER-AUDIT-2026-09-05.md` and were
run against source revision `d349e23`. Run from the repository root with the
project's Go and Node runtimes:

```sh
rtk proxy go run docs/audits/2026-09-05/reproduce.go
rtk proxy node docs/audits/2026-09-05/planning-ui.mjs
rtk proxy node docs/audits/2026-09-05/execution-ui.mjs
rtk proxy node docs/audits/2026-09-05/measure.mjs
rtk proxy node docs/audits/2026-09-05/time-basis.mjs
```

The Go probe calls the real import, scheduling, comparison and execution-evidence
functions with generic inputs. JavaScript probes call pure exports or extract
the actual UI handlers into a VM with controlled request timing and DOM stubs.
They print diagnostic observations; they are not pass/fail acceptance tests or
full-browser validation. Extraction boundaries may need updating after refactors.
None of these probes contacts a server or changes stored plans.

The markup probe uses inert formatting elements only. It demonstrates that
labels are emitted as HTML; it does not execute a script or establish a complete
exploit against a deployed application.

`observations.txt` captures the combined output from this audit. Correcting a
defect should change the corresponding output; do not treat the historical
buggy output as expected product behavior. Add acceptance regressions alongside
the eventual fixes.
