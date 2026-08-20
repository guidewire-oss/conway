// Command fixgen writes tests/fixtures/schedule-demo.json: the demo plan's
// execution order, straight from the scheduler.
//
// The fixture exists so the JS view in app/js/order.js is tested against real
// server output rather than a hand-written sample that can drift from the Go
// types. This program is committed rather than described in prose, because a
// regeneration step nobody can run is a fixture nobody will regenerate.
//
// Run it from the repository root:
//
//	go run ./tools/fixgen
//
// The destination is fixed rather than taken as an argument: there is exactly one
// fixture, and a path from the command line only invites writing it somewhere the
// tests do not read.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"conway/server/planning"
)

const fixturePath = "tests/fixtures/schedule-demo.json"

func main() {
	teams, inits := planning.Demo()
	sched := planning.ComputeSchedule(teams, inits,
		planning.Params{HorizonWeeks: 26, CapacityLoss: 0.1}, planning.DemoScheduling())

	blob, err := json.MarshalIndent(sched, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixgen: marshal:", err)
		os.Exit(1)
	}
	// 0600: git records only the executable bit for a tracked file, so the on-disk
	// mode of a regenerated fixture is nobody's business but this process's.
	if err := os.WriteFile(fixturePath, append(blob, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "fixgen: write:", err, "— run this from the repository root")
		os.Exit(1)
	}
	fmt.Printf("fixgen: wrote %s (%d initiatives, %d pods, rule %q)\n",
		fixturePath, len(sched.Initiatives), len(sched.PodWeeks), sched.Rule)
}
