# Test fixtures

## `schedule-demo.json`

Real output from the Go scheduler for the demo plan — not a hand-written sample.
`tests/order.test.mjs` renders the view against it, so the JS view and the Go
`planning.Schedule` shape are checked against each other rather than against two
independent guesses that can drift apart.

Regenerate it whenever the `Schedule` shape or the demo data changes:

```go
// go run ./scratch/fixgen.go tests/fixtures/schedule-demo.json
teams, inits := planning.Demo()
s := planning.ComputeSchedule(teams, inits,
    planning.Params{HorizonWeeks: 26, CapacityLoss: 0.1}, planning.DemoScheduling())
b, _ := json.MarshalIndent(s, "", "  ")
os.WriteFile(os.Args[1], append(b, '\n'), 0o644)
```

`TestPlanningSuite`'s "the committed fixture still matches this package's shape"
spec fails if the Go side changes a field the fixture names, which is the signal
to regenerate — the JS tests cannot see a Go rename on their own.
