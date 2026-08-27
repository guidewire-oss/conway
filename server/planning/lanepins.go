package planning

import (
	"fmt"
)

// ValidateLanePins (spec 008 Decision 3): refuse a vertical drop whose lanes
// another slice already occupies in any overlapping week. The reference stack
// is the CURRENT schedule (pre-edit initiatives) — the drop is judged against
// the world the planner is looking at.
func ValidateLanePins(edited []Initiative, edits []InitiativeEdit, sp SchedulingParams, horizonWeeks float64, teams []Team) string {
	// Only edits that set lane pins participate.
	pinning := map[string]map[string]int{}
	for _, e := range edits {
		if e.PinnedLanes != nil {
			pinning[e.Name] = *e.PinnedLanes
		}
	}
	if len(pinning) == 0 {
		return ""
	}
	// Schedule with pins REMOVED (the pre-drop world), lane-packed like the view.
	pre := make([]Initiative, 0, len(edited))
	for _, it := range edited {
		cp := it
		cp.PinnedLanes = nil
		cp.PinnedStarts = nil
		pre = append(pre, cp)
	}
	sched := ComputeScheduleWith(teams, pre, Params{HorizonWeeks: horizonWeeks, CapacityLoss: 0.1}, sp, ScheduleOptions{CompareWipModels: false})
	for name, pins := range pinning {
		var mine *ScheduledInitiative
		for i := range sched.Initiatives {
			if sched.Initiatives[i].Name == name {
				mine = &sched.Initiatives[i]
			}
		}
		if mine == nil {
			continue
		}
		for pod, lane := range pins {
			for _, s := range mine.Slices {
				if s.Pod != pod {
					continue
				}
				// Any other slice in the pod overlapping [start,finish)
				// whose packed lanes intersect [lane, lane+width)?
				for _, other := range sched.Initiatives {
					if other.Name == name {
						continue
					}
					for _, os := range other.Slices {
						if os.Pod != pod || os.FinishWeek <= s.StartWeek || os.StartWeek >= s.FinishWeek {
							continue
						}
						ow := os.LanesUsed
						if ow < 1 {
							ow = 1
						}
						// The view packs other slices from lane 0 upward in
						// start order; a conservative check: any overlapping
						// slice occupies [0, ow). If the pin's range touches
						// it, refuse.
						if lane < ow {
							return fmt.Sprintf("%s: track %d at %s overlaps %s (w%d–w%d) — pick a free track",
								name, lane+1, pod, other.Name, os.StartWeek, os.FinishWeek)
						}
					}
				}
			}
		}
	}
	return ""
}
