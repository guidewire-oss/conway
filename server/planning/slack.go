package planning

// Slice slack: the latest-start and slack pair FR-041 requires, computed as a
// critical-path backward pass over each initiative's slices.
//
// Latest start is defined by the spec — "the last week the slice can begin
// without moving its initiative's commit date" — and the commit date moves when
// the raw finish moves (the buffer is a flat percentage of the chain, Decision
// 20, so it never absorbs a late slice), so the pass anchors on rawFinishWeek:
//
//	LF(terminal slice) = the initiative's rawFinishWeek
//	LF(S)              = min over successors T of LS(T)
//	LS(S)              = LF(S) - duration(S)
//	slack(S)           = LS(S) - startWeek(S)
//
// Slices arrive in dependency order (podOrder's topological sort), so one
// reverse walk is the whole pass — no recursion, no re-sorting.
//
// Two properties hold structurally and are asserted in the specs:
//   - slack is never negative: a successor never starts before its predecessor
//     finishes, so LF(S) >= finish(S) and LS(S) >= start(S). A slice that
//     already waited for capacity has that waiting priced into the finish, and
//     reads as zero slack — "you can wait no more", which is the honest answer.
//   - LS never exceeds the raw finish: the terminal anchor is the raw finish
//     itself, and every LF is a min over chains that end there. Slack can never
//     eat the buffer's weeks, because the buffer is not on the slice graph.

// annotateSliceSlack fills LatestStartWeek and SlackWeeks on every slice of
// every initiative, in place. It runs after the winning schedule is chosen:
// slack is a property of the order as computed, not of the input.
func annotateSliceSlack(sis []ScheduledInitiative) {
	for i := range sis {
		si := &sis[i]
		slices := si.Slices
		if len(slices) == 0 {
			continue
		}

		// Successors: pod -> the slices whose DependsOn names it. DependsOn is
		// the pruned, in-plan edge list the schedule itself sequenced by, so
		// the backward pass walks exactly the arrows the timeline draws.
		successors := map[string][]int{}
		for j, sl := range slices {
			for _, dep := range sl.DependsOn {
				successors[dep] = append(successors[dep], j)
			}
		}

		// latestFinish per slice index; terminal slices anchor on the raw finish.
		latestFinish := make([]int, len(slices))
		for j := len(slices) - 1; j >= 0; j-- {
			sl := slices[j]
			lf := si.RawFinishWeek
			if succs := successors[sl.Pod]; len(succs) > 0 {
				// Slices are in dependency order, so every successor has a
				// later index and its latest start is already computed.
				for _, t := range succs {
					if ls := latestFinish[t] - (slices[t].FinishWeek - slices[t].StartWeek); ls < lf {
						lf = ls
					}
				}
			}
			latestFinish[j] = lf
			dur := sl.FinishWeek - sl.StartWeek
			slices[j].LatestStartWeek = lf - dur
			slices[j].SlackWeeks = slices[j].LatestStartWeek - sl.StartWeek
		}
	}
}

// propagateSliceSlack copies the slack pair onto the per-pod schedule's slice
// copies. PodWeeks is built during the run, before annotateSliceSlack runs, and
// the pod view reads those copies — FR-041's "start by" belongs there just as
// much as on the initiative's own slices, so the two must never disagree.
func propagateSliceSlack(sis []ScheduledInitiative, pods []PodSchedule) {
	type key struct{ initiative, pod string }
	annotated := map[key]WorkSlice{}
	for _, si := range sis {
		for _, sl := range si.Slices {
			annotated[key{si.Name, sl.Pod}] = sl
		}
	}
	for i := range pods {
		for j := range pods[i].Slices {
			sl := &pods[i].Slices[j]
			if a, ok := annotated[key{sl.Initiative, sl.Pod}]; ok {
				sl.LatestStartWeek, sl.SlackWeeks = a.LatestStartWeek, a.SlackWeeks
			}
		}
	}
}
