package planning

// Story 5: priced remedies for a date that will not fit (AC 5.1-5.5, FR-015),
// and AC 3.3's conflicting-commitment pairs.
//
// Every remedy is a proposal, nothing more: it is computed by trial — apply the
// change to a copy of the inputs, recompute the whole schedule, diff it against
// the base — so the verdict, the objective delta and the named victims are what
// the engine actually measured, never a formula guessing at them (FR-022: the
// plan itself is never touched).
//
// Transfer-capacity is deliberately absent: Decision 7 holds it back until
// §10 Q1's site-overlap factor is decided, which is why this file generates no
// transfer remedies at all rather than inventing a plausibility number.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Remedy is one priced option for rescuing a missed date (§7). Magnitude means
// different things per kind — the priority to raise to, the fraction of work to
// descope, the tracks to add, or the weeks to relax a date by — because the
// spec's shape is one magnitude field, and a union of typed magnitudes would
// make the JSON harder, not easier, to read in a network tab.
type Remedy struct {
	Kind      string  `json:"kind"`          // raise-priority | descope | add-capacity | transfer-capacity | relax-date | defer-other | unlock
	Target    string  `json:"target"`        // the initiative being rescued
	Pod       string  `json:"pod,omitempty"` // add-capacity: where the tracks go
	Magnitude float64 `json:"magnitude"`     // per-kind; see the type comment
	// Plausibility is carried for transfer-capacity only (Decision 7). No remedy
	// in this file sets it; the field exists so the §7 shape is complete and a
	// future transfer remedy does not break the response contract.
	Plausibility        float64        `json:"plausibility,omitempty"`
	ResultingVerdict    string         `json:"resultingVerdict"`              // the target's verdict under the remedy
	TargetWeeksLate     int            `json:"targetWeeksLate"`               // ...and its remaining lateness
	ObjectiveDelta      float64        `json:"objectiveDelta"`                // what it does to the whole portfolio (FR-015)
	AffectedInitiatives []RemedyEffect `json:"affectedInitiatives,omitempty"` // the victims, with week deltas
	Note                string         `json:"note,omitempty"`
}

// RemedyEffect is one other initiative's movement under a remedy. Only the
// moved are listed: an initiative whose weeks did not change is not a cost of
// the decision, and padding the list with zeroes would bury the names that
// matter (AC 5.2's "victims") in noise.
type RemedyEffect struct {
	Initiative       string `json:"initiative"`
	StartDeltaWeeks  int    `json:"startDeltaWeeks"`
	CommitDeltaWeeks int    `json:"commitDeltaWeeks"`
}

// Conflict is one conflicting-commitments pair (AC 3.3): two date-locked
// initiatives contending for the same pod's tracks in the same window, at least
// one of them missing because of it. Surfaced, never silently resolved — the
// engine will not unlock either side on its own.
type Conflict struct {
	A    string `json:"a"`
	B    string `json:"b"`
	Pod  string `json:"pod"`
	Note string `json:"note,omitempty"`
}

// remedyKinds are the enum values the API promises (§7). transfer-capacity is
// listed for completeness and never generated here (see the file comment).
const (
	remedyRaisePriority = "raise-priority"
	remedyDescope       = "descope"
	remedyAddCapacity   = "add-capacity"
	remedyRelaxDate     = "relax-date"
	remedyDeferOther    = "defer-other"
	remedyUnlock        = "unlock"
)

// TransferDeferredWarning is the response warning that says why transfer-capacity
// remedies are absent (Decision 7 defers them until §10 Q1 is decided). Exported
// because two emitters must say the same thing — the endpoint and the fixture
// generator — and a duplicated literal is how they drift.
const TransferDeferredWarning = "transfer-capacity remedies are not offered yet: the site-overlap factor is undecided (spec 001 §10 Q1, Decision 7)"

// ComputeRemedies prices the rescue options for the named targets. A nil or
// empty target list means every initiative that missed its date, which is
// FR-015's "for each missed date"; naming an on-time initiative yields nothing,
// because there is nothing to rescue. The inputs are never modified.
func ComputeRemedies(teams []Team, inits []Initiative, params Params, sp SchedulingParams, targets []string) []Remedy {
	base := computeOne(teams, inits, params, sp)
	baseBy := map[string]ScheduledInitiative{}
	for _, si := range base.Initiatives {
		baseBy[si.Name] = si
	}

	names := append([]string(nil), targets...) // never write through the caller's slice
	if len(names) == 0 {
		for _, si := range base.Initiatives {
			if missesDate(si.Verdict) {
				names = append(names, si.Name)
			}
		}
	}

	var out []Remedy
	for _, name := range names {
		si, ok := baseBy[name]
		if !ok || !missesDate(si.Verdict) {
			continue // unknown, or nothing to rescue
		}
		out = append(out, remediesFor(teams, inits, params, sp, base, si)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ObjectiveDelta < out[j].ObjectiveDelta
	})
	return out
}

// remediesFor generates the candidate remedies for one missed date. Most
// candidates are only emitted when the trial actually improves the target's
// outcome — a descope that changes nothing is not an option, it is noise. The
// one exception is unlock, emitted for every DateLocked target: AC 3.3 requires
// the relaxation to be offered even when releasing the lock does not improve
// the verdict, because the offer is the point.
func remediesFor(teams []Team, inits []Initiative, params Params, sp SchedulingParams,
	base *Schedule, target ScheduledInitiative) []Remedy {

	// trial applies a candidate to copies of the inputs and measures it against
	// the base schedule. The copies are deep (whole struct + Work map), per the
	// standing lesson about field-listing copies dropping sequencing attributes.
	trial := func(edit func([]Team, []Initiative) ([]Team, []Initiative)) (*Schedule, ScheduledInitiative, []RemedyEffect) {
		i2 := copyInitiatives(inits)
		t2 := append([]Team(nil), teams...) // Team holds no references; value copy is a clone
		if edit != nil {
			t2, i2 = edit(t2, i2)
		}
		s := computeOne(t2, i2, params, sp)
		var after ScheduledInitiative
		for _, si := range s.Initiatives {
			if si.Name == target.Name {
				after = si
				break
			}
		}
		cmp := CompareToBaseline(base, s)
		var moved []RemedyEffect
		for _, d := range cmp.Initiatives {
			if d.Name == target.Name {
				continue
			}
			if d.StartDeltaWeeks != 0 || d.CommitDeltaWeeks != 0 {
				moved = append(moved, RemedyEffect{
					Initiative:       d.Name,
					StartDeltaWeeks:  d.StartDeltaWeeks,
					CommitDeltaWeeks: d.CommitDeltaWeeks,
				})
			}
		}
		return s, after, moved
	}

	// The pod capacity remedies act at: the constraint the schedule itself
	// blamed, when it blamed a pod; otherwise the drum. Computed once, before
	// the trial helper, because the remedy constructor reports it.
	pod := bindingPodOf(base, target)

	newRemedy := func(kind, note string, mag float64, s *Schedule, after ScheduledInitiative, moved []RemedyEffect) Remedy {
		return Remedy{
			Kind: kind, Target: target.Name, Magnitude: mag, Pod: remedyPod(kind, pod),
			ResultingVerdict: after.Verdict, TargetWeeksLate: after.WeeksLate,
			ObjectiveDelta:      round1(s.ObjectiveScore - base.ObjectiveScore),
			AffectedInitiatives: moved, Note: note,
		}
	}

	var out []Remedy

	// The schedule reports stated rank, not stated priority; the raise search
	// needs the priority off the initiative itself.
	var statedPriority int
	for _, it := range inits {
		if it.Name == target.Name {
			statedPriority = it.StatedPriority
			break
		}
	}

	// raise-priority (AC 5.2): find a strictly earlier priority that lands the
	// date, honouring it the way the engine honours every priority that must
	// hold — as a lock. Only locked priorities are guaranteed a slot across all
	// dispatch rules; an unlocked one is merely advice the winning rule may take.
	if statedPriority > 1 {
		works := func(p int) (*Schedule, ScheduledInitiative, []RemedyEffect, bool) {
			s, after, moved := trial(func(t2 []Team, i2 []Initiative) ([]Team, []Initiative) {
				for j := range i2 {
					if i2[j].Name == target.Name {
						i2[j].StatedPriority = p
						i2[j].PriorityLocked = true
					}
				}
				return t2, i2
			})
			return s, after, moved, after.Verdict == verdictOnTime
		}
		// Priority 1 is the earliest slot any rule can give; if that does not
		// land the date, no raise will, and offering one would be a guess.
		if s, after, moved, ok := works(1); ok {
			best := 1
			bs, ba, bm := s, after, moved
			// Bisect for the highest priority that still works: the least
			// disruptive raise. Bisection assumes raising is monotone, which the
			// engine only approximately guarantees; every reported remedy is
			// still one the trial verified, so a non-monotone plan can cost us
			// the optimal number, never the truth of the one we report.
			lo, hi := 1, statedPriority-1
			for lo < hi {
				mid := (lo + hi + 1) / 2
				if s2, a2, m2, ok2 := works(mid); ok2 {
					lo, best, bs, ba, bm = mid, mid, s2, a2, m2
				} else {
					hi = mid - 1
				}
			}
			note := fmt.Sprintf("priority %d lands it; %d other initiative(s) move", best, len(bm))
			out = append(out, newRemedy(remedyRaisePriority, note, float64(best), bs, ba, bm))
		}
	}

	// descope: a ladder of fractions, each emitted only when it measurably
	// improves the target. The trial reuses the lever the Network view already
	// applies, so a remedy and a manual lever cannot disagree about semantics.
	for _, frac := range []float64{0.25, 0.5} {
		s, after, moved := trial(func(t2 []Team, i2 []Initiative) ([]Team, []Initiative) {
			t, i, _ := applyLevers(t2, i2, []Lever{{Type: "descope", Initiative: target.Name, N: frac}})
			return t, i
		})
		if improvesOutcome(target, after) {
			out = append(out, newRemedy(remedyDescope, fmt.Sprintf("cut %d%% of the estimated work", int(frac*100)), frac, s, after, moved))
		}
	}

	// add-capacity: tracks at the pod that binds the target — the constraint
	// the schedule itself named, falling back to the drum when the miss is
	// release-bound rather than pod-bound. The pod itself was resolved above.
	for _, n := range []float64{1, 2} {
		s, after, moved := trial(func(t2 []Team, i2 []Initiative) ([]Team, []Initiative) {
			t, i, _ := applyLevers(t2, i2, []Lever{{Type: "addCapacity", Pod: pod, N: n}})
			return t, i
		})
		if improvesOutcome(target, after) {
			out = append(out, newRemedy(remedyAddCapacity, fmt.Sprintf("+%d track(s) at %s", int(n), pod), n, s, after, moved))
		}
	}

	// relax-date: the earliest date the current plan can actually meet — the
	// buffered commit week, not the raw finish (Decision 9). By construction
	// this lands on-time, but it is still verified by trial so the victims and
	// the objective it reports are measured.
	if target.TargetWeek != nil {
		relaxTo := target.CommitWeek
		if d := dateAtWeek(sp.PeriodStart, relaxTo); d != "" {
			s, after, moved := trial(func(t2 []Team, i2 []Initiative) ([]Team, []Initiative) {
				for j := range i2 {
					if i2[j].Name == target.Name {
						i2[j].TargetDate = d
					}
				}
				return t2, i2
			})
			if after.Verdict == verdictOnTime {
				mag := float64(relaxTo - *target.TargetWeek)
				out = append(out, newRemedy(remedyRelaxDate,
					fmt.Sprintf("move the date to week %d (%s)", relaxTo, d), mag, s, after, moved))
			}
		}
	}

	// defer-other: deferring the initiatives queued at the binding pod ahead of
	// the target. Candidates come from the base schedule's slices, because the
	// blocker is whoever holds the tracks the target is waiting for — a pod
	// window that has already closed cannot be freed by deferring someone else.
	for _, cand := range deferCandidates(base, target, pod) {
		s, after, moved := trial(func(t2 []Team, i2 []Initiative) ([]Team, []Initiative) {
			t, i, _ := applyLevers(t2, i2, []Lever{{Type: "defer", Initiative: cand}})
			return t, i
		})
		if improvesOutcome(target, after) {
			out = append(out, newRemedy(remedyDeferOther,
				fmt.Sprintf("defer %s out of this period", cand), 1, s, after, moved))
		}
	}

	// unlock (AC 3.3's relaxation for a locked commitment): release the date
	// lock, keep the date as a target. This does not pretend to make the date —
	// it trades the commitment away, and the objective delta prices exactly
	// that trade (lock dominance is the objective's way of making a broken
	// commitment the most expensive thing there is).
	if target.DateLocked {
		s, after, moved := trial(func(t2 []Team, i2 []Initiative) ([]Team, []Initiative) {
			for j := range i2 {
				if i2[j].Name == target.Name {
					i2[j].DateLocked = false
				}
			}
			return t2, i2
		})
		out = append(out, newRemedy(remedyUnlock,
			"release the date lock; the date stays as a target", 1, s, after, moved))
	}

	return out
}

// improvesOutcome reports whether the target is strictly better off: a better
// verdict, or the same verdict with less lateness.
func improvesOutcome(before, after ScheduledInitiative) bool {
	rb, ra := verdictRank(before.Verdict), verdictRank(after.Verdict)
	if ra != rb {
		return ra < rb
	}
	return after.WeeksLate < before.WeeksLate
}

func verdictRank(v string) int {
	switch v {
	case verdictOnTime:
		return 0
	case verdictLate:
		return 1
	case verdictInfeasible:
		return 2
	}
	return 3
}

// bindingPodOf names the pod to add capacity at: the pod of the target's
// longest-waiting slice, because that wait is the delay capacity would remove.
// BindingConstraint cannot be used here — it carries the reason ("pod-capacity"),
// never a pod's name — and the drum is only the fallback for a release-bound
// miss, where the constraint the release staggers against is the right place.
func bindingPodOf(base *Schedule, target ScheduledInitiative) string {
	best, bestWait := "", -1.0
	for _, s := range target.Slices {
		if s.WaitWeeks > bestWait {
			best, bestWait = s.Pod, s.WaitWeeks
		}
	}
	if bestWait > 0 {
		return best
	}
	if len(base.DrumPods) > 0 {
		return base.DrumPods[0]
	}
	return best
}

// deferCandidates lists the initiatives whose deferral could free the target's
// window: anyone with a slice at the binding pod that has not finished by the
// target's start. When the miss is not bound to a pod, the queue ahead of the
// target's commit is the honest candidate set.
func deferCandidates(base *Schedule, target ScheduledInitiative, pod string) []string {
	var out []string
	seen := map[string]bool{target.Name: true}
	if pod != "" && pod != "wip-limit" {
		for _, si := range base.Initiatives {
			if seen[si.Name] {
				continue
			}
			for _, s := range si.Slices {
				// Abutting counts: the predecessor whose slice ends the week the
				// target starts is holding the tracks the target is waiting for,
				// whole-week packing just makes the handoff look disjoint.
				if s.Pod == pod && s.FinishWeek >= target.StartWeek && s.StartWeek <= target.RawFinishWeek {
					out = append(out, si.Name)
					seen[si.Name] = true
					break
				}
			}
		}
		return out
	}
	for _, si := range base.Initiatives {
		if !seen[si.Name] && si.CommitWeek <= target.CommitWeek {
			out = append(out, si.Name)
			seen[si.Name] = true
		}
	}
	return out
}

// dateAtWeek is weekOf's inverse: the ISO date of a week index from the period
// start. An unparseable period start yields "" (no date), which the caller
// treats as "cannot offer a relax-date remedy" rather than inventing one.
func dateAtWeek(periodStart string, w int) string {
	if strings.TrimSpace(periodStart) == "" {
		return ""
	}
	t0, err := time.Parse(isoDate, strings.TrimSpace(periodStart))
	if err != nil {
		return ""
	}
	return t0.AddDate(0, 0, w*7).Format(isoDate)
}

// conflictingCommitments surfaces AC 3.3's pairs: two date-locked initiatives
// whose spans at a shared pod overlap or abut, with at least one of them
// missing its date. Abutting counts because whole-week packing makes a
// queue-predecessor look disjoint — the second initiative is waiting for the
// tracks the first is still on. The pod is not attributed through
// BindingConstraint (which carries the reason, "pod-capacity", not the pod);
// contention is read from the slices themselves.
func conflictingCommitments(sis []ScheduledInitiative) []Conflict {
	type span struct{ start, finish int }
	spans := map[string]map[string]span{} // pod -> initiative -> window
	verdicts := map[string]string{}
	for _, si := range sis {
		if !si.DateLocked {
			continue
		}
		verdicts[si.Name] = si.Verdict
		for _, s := range si.Slices {
			if spans[s.Pod] == nil {
				spans[s.Pod] = map[string]span{}
			}
			w, ok := spans[s.Pod][si.Name]
			if !ok || s.StartWeek < w.start {
				w.start = s.StartWeek
			}
			if s.FinishWeek > w.finish {
				w.finish = s.FinishWeek
			}
			spans[s.Pod][si.Name] = w
		}
	}

	var out []Conflict
	for pod, byInit := range spans {
		names := make([]string, 0, len(byInit))
		for n := range byInit {
			names = append(names, n)
		}
		sort.Strings(names)
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				a, b := names[i], names[j]
				wa, wb := byInit[a], byInit[b]
				if wa.start > wb.finish || wb.start > wa.finish {
					continue // windows neither overlap nor abut: no shared contention
				}
				if !missesDate(verdicts[a]) && !missesDate(verdicts[b]) {
					continue // both hold; whatever either misses, it is not this pair
				}
				out = append(out, Conflict{
					A: a, B: b, Pod: pod,
					Note: fmt.Sprintf("both date-locked; contend for %s in weeks %d-%d",
						pod, minInt(wa.start, wb.start), maxInt(wa.finish, wb.finish)),
				})
			}
		}
	}
	// The pods come from a map, so without this the same schedule could report
	// its conflicts in a different order on a different boot — and Schedule is
	// deterministic by contract (AC 1.4).
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pod != out[j].Pod {
			return out[i].Pod < out[j].Pod
		}
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	return out
}

func missesDate(v string) bool {
	return v == verdictLate || v == verdictInfeasible
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// remedyPod names the pod field only where a pod is part of the remedy's
// action; a relax-date or an unlock is not about a pod, and an empty field
// is the honest value there.
func remedyPod(kind, pod string) string {
	if kind == remedyAddCapacity {
		return pod
	}
	return ""
}
