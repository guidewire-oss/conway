# Test fixtures

## `schedule-demo.json`

Real output from the Go scheduler for the demo plan — not a hand-written sample.
`tests/order.test.mjs` renders the view against it, so the JS view and the Go
`planning.Schedule` shape are checked against each other rather than against two
independent guesses that can drift apart.

Regenerate it whenever the `Schedule` shape or the demo data changes:

```sh
go run ./tools/fixgen   # from the repository root
```

`TestPlanningSuite`'s "still matches the committed fixture the JS view is tested
against" spec decodes this file with `DisallowUnknownFields` and fails when it
stops describing the package. That failure is the signal to run the command
above — the JS tests cannot see a Go-side rename on their own.
