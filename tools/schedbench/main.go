// schedbench times ComputeSchedule against a real plan payload, so a claim about
// scheduling cost is measured on the plan that was slow rather than on Demo data.
//
// Feed it the body of GET /api/plan/{id}:
//
//	go run ./tools/schedbench /tmp/plan.json
//
// It reports the cost with and without the WIP-model comparison (D22 as amended),
// which is the difference between four full schedules per request and one.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"conway/server/planning"
)

type planPayload struct {
	HorizonWeeks float64                   `json:"horizonWeeks"`
	CapacityLoss float64                   `json:"capacityLoss"`
	Teams        []planning.Team           `json:"teams"`
	Initiatives  []planning.Initiative     `json:"initiatives"`
	Scheduling   planning.SchedulingParams `json:"scheduling"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: schedbench <plan.json>")
		os.Exit(2)
	}
	// The path is this tool's entire interface: a developer naming a plan payload on
	// their own machine. There is no untrusted caller to traverse anything.
	blob, err := os.ReadFile(os.Args[1]) // #nosec G304,G703 -- operator-supplied path, dev tool
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var p planPayload
	if err := json.Unmarshal(blob, &p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	params := planning.Params{HorizonWeeks: p.HorizonWeeks, CapacityLoss: p.CapacityLoss}
	fmt.Printf("teams=%d initiatives=%d horizon=%.0fw capacityLoss=%.2f\n",
		len(p.Teams), len(p.Initiatives), p.HorizonWeeks, p.CapacityLoss)

	run := func(label string, opts planning.ScheduleOptions) *planning.Schedule {
		start := time.Now()
		sched := planning.ComputeScheduleWith(p.Teams, p.Initiatives, params, p.Scheduling, opts)
		took := time.Since(start)
		out, _ := json.Marshal(sched)
		fmt.Printf("%-28s %8.2fs  %9d bytes json\n", label, took.Seconds(), len(out))
		return sched
	}

	with := run("with wip comparison", planning.ScheduleOptions{CompareWipModels: true})
	without := run("without wip comparison", planning.ScheduleOptions{})

	last := 0
	for _, si := range without.Initiatives {
		if si.CommitWeek > last {
			last = si.CommitWeek
		}
	}
	cells := 0
	for _, ps := range without.PodWeeks {
		cells += len(ps.Weeks)
	}
	fmt.Printf("last commit week=%d  podWeek cells=%d  wipModels=%d\n",
		last, cells, len(with.WipModels))
}
