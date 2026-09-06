package planning

import (
	"fmt"
	"sort"
)

// ValidateLanePins retains the legacy default-loss API. Callers with plan
// inputs must use ValidateLanePinsWithParams to preserve the actual assumptions.
func ValidateLanePins(edited []Initiative, _ []InitiativeEdit, sp SchedulingParams, horizonWeeks float64, teams []Team) string {
	return ValidateLanePinsWithParams(edited, sp, Params{HorizonWeeks: horizonWeeks, CapacityLoss: 0.1}, teams)
}

// ValidateLanePinsWithParams reserves every fixed pin against the edited
// schedule's occupied weeks. Unpinned work is free to use the remaining lanes.
// specs/019-scheduling-audit-and-gantt-integrity.md:184
func ValidateLanePinsWithParams(edited []Initiative, sp SchedulingParams, params Params, teams []Team) string {
	pins := map[string]map[string]int{}
	tracks := map[string]int{}
	for _, team := range teams {
		tracks[team.Name] = team.EffectiveTracks()
	}
	for _, it := range edited {
		pods := make([]string, 0, len(it.PinnedLanes))
		for pod := range it.PinnedLanes {
			pods = append(pods, pod)
		}
		sort.Strings(pods)
		for _, pod := range pods {
			offset := it.PinnedLanes[pod]
			if offset < 0 || offset >= tracks[pod] {
				return fmt.Sprintf("%s: track %d at %s is outside its %d physical tracks", it.Name, offset+1, pod, tracks[pod])
			}
			if pins[pod] == nil {
				pins[pod] = map[string]int{}
			}
			pins[pod][it.Name] = offset
		}
	}
	if len(pins) == 0 {
		return ""
	}
	if err := ValidateInitiativeNames(edited); err != nil {
		return err.Error()
	}
	sched := ComputeScheduleWith(teams, edited, params, sp, ScheduleOptions{})
	for _, pod := range sched.PodWeeks {
		fixed := pins[pod.Pod]
		if len(fixed) == 0 {
			continue
		}
		work := map[string]WorkSlice{}
		for _, sl := range pod.Slices {
			work[sl.Initiative] = sl
			if offset, ok := fixed[sl.Initiative]; ok && offset+maxInt(1, sl.LanesUsed) > pod.Tracks {
				return fmt.Sprintf("%s: track %d and its initial width at %s exceed %d physical tracks", sl.Initiative, offset+1, pod.Pod, pod.Tracks)
			}
		}
		for _, week := range pod.Weeks {
			reserved := map[int]string{}
			for _, name := range week.Initiatives {
				offset, fixedHere := fixed[name]
				if !fixedHere {
					continue
				}
				sl := work[name]
				width := maxInt(1, sl.LanesUsed)
				if len(sl.Phases) > 0 {
					width = 0
					for _, phase := range sl.Phases {
						if week.Week >= phase.FromWeek && week.Week < phase.ToWeek {
							width = phase.Lanes
							break
						}
					}
				}
				if width > pod.Tracks {
					return fmt.Sprintf("%s: its occupied width at %s exceeds %d physical tracks in week %d", name, pod.Pod, pod.Tracks, week.Week)
				}
				// Growth uses the chart's offset clamp; the saved initial lane
				// does not create tracks beyond the team's physical capacity.
				offset = maxInt(0, minInt(offset, pod.Tracks-width))
				for lane := offset; lane < offset+width; lane++ {
					if other, occupied := reserved[lane]; occupied {
						return fmt.Sprintf("%s: track %d at %s overlaps %s in week %d; pick a free track", name, lane+1, pod.Pod, other, week.Week)
					}
					reserved[lane] = name
				}
			}
		}
	}
	return ""
}
