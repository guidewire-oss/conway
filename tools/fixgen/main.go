// Command fixgen writes the JS view fixtures: the demo plan's execution order
// and its priced remedies, straight from the Go code.
//
// The fixtures exist so the JS views in app/js are tested against real server
// output rather than hand-written samples that can drift from the Go types.
// This program is committed rather than described in prose, because a
// regeneration step nobody can run is a fixture nobody will regenerate.
//
// Run it from the repository root:
//
//	go run ./tools/fixgen
//
// The destinations are fixed rather than taken as arguments: there are exactly
// two fixtures, and paths from the command line only invite writing them
// somewhere the tests do not read.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"conway/server/planning"
)

const fixturePath = "tests/fixtures/schedule-demo.json"
const remediesPath = "tests/fixtures/remedies-demo.json"

func main() {
	teams, inits := planning.Demo()
	params := planning.Params{HorizonWeeks: 26, CapacityLoss: 0.1}
	sp := planning.DemoScheduling()

	sched := planning.ComputeSchedule(teams, inits, params, sp)
	write(fixturePath, sched, func() string {
		return fmt.Sprintf("fixgen: wrote %s (%d initiatives, %d pods, rule %q)\n",
			fixturePath, len(sched.Initiatives), len(sched.PodWeeks), sched.Rule)
	})

	// The remedies fixture keeps the whole response envelope — warnings included —
	// because the JS view is tested against what the endpoint actually says,
	// including the deliberate absence of transfer-capacity.
	remedies := planning.ComputeRemedies(teams, inits, params, sp, nil)
	envelope := map[string]any{
		"remedies": remedies,
		"warnings": []string{
			"transfer-capacity remedies are not offered yet: the site-overlap factor is undecided (spec 001 §10 Q1, Decision 7)",
		},
	}
	write(remediesPath, envelope, func() string {
		return fmt.Sprintf("fixgen: wrote %s (%d remedies)\n", remediesPath, len(remedies))
	})
}

func write(path string, v any, summarize func() string) {
	blob, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixgen: marshal:", err)
		os.Exit(1)
	}
	// 0600: git records only the executable bit for a tracked file, so the on-disk
	// mode of a regenerated fixture is nobody's business but this process's.
	if err := os.WriteFile(path, append(blob, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "fixgen: write:", err, "— run this from the repository root")
		os.Exit(1)
	}
	fmt.Print(summarize())
}
