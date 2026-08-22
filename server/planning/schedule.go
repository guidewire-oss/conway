package planning

// Execution-order scheduling: specs/001-plan-execution-order.md.
//
// The flat-rho model in simulate.go answers "how full is this pod over the
// period". It has no time axis, so it cannot answer "will this date hold", which
// is what this file adds: weekly buckets, pods as finite servers whose tracks
// are the servers, and one work slice per initiative x pod occupying one track
// for its duration (Decision 1, and Q4 — a slice never spans two tracks).
//
// Weeks are integers counted from the period start, and a finish week is
// exclusive: a 6-week slice starting in week 5 occupies weeks 5..10 and finishes
// in week 11. That makes a successor's earliest start exactly its predecessor's
// finish week, with no off-by-one to carry around.
//
// The queue multiplier m(rho) is deliberately absent here. Under finite capacity
// waiting is produced by the schedule rather than modelled by a formula, so
// applying it too would count the same delay twice (Decision 4).

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Verdict values, per §7. "at-risk" belongs to the actuals view, where status
// comes from buffer consumption rather than a plan-time comparison (Decision
// 17), so nothing here emits it.
const (
	verdictOnTime        = "on-time"
	verdictLate          = "late"
	verdictNoDate        = "no-date"
	verdictInfeasible    = "structurally-infeasible"
	verdictUnschedulable = "unschedulable"
)

// bindingConstraint values, per §7. Exactly one of these explains why a slice or
// an initiative starts when it does; FR-021 makes that explanation mandatory,
// because a start week without a reason reads as a bug.
const (
	bindDependency    = "dependency"
	bindPodCapacity   = "pod-capacity"
	bindLead          = "lead"
	bindWipLimit      = "wip-limit"     // the org limit
	bindPodWipLimit   = "pod-wip-limit" // the per-pod concurrency cap
	bindStartsCap     = "starts-cap"    // the change-absorption cap (FR-026)
	bindKitGate       = "kit-gate"
	bindEarliestStart = "earliest-start"
	bindPredecessor   = "predecessor"
)

// Dispatch rules, run in this order. D6 keeps the best by objective; ties keep
// the earlier rule, which is what makes the winner deterministic (AC 1.4).
var dispatchRules = []string{
	ruleTardinessCost,
	"minimum-slack",
	"value-per-constraint-week",
	"constraint-first",
	ruleStatedPriority,
}

const (
	ruleTardinessCost  = "tardiness-cost"
	ruleStatedPriority = "stated-priority"
)

const (
	// defaultBufferPct is a quarter of the chain (Decision 20 as amended
	// 2026-08-19): safe estimates and capacityLoss already pad the same risk, so
	// the classic 50% CCPM buffer would be the third layer on one uncertainty.
	defaultBufferPct = 0.25
	// defaultLookaheadK scales the slack discount in the tardiness index. The
	// knob Decision 2 predicted; 2.0 is the value the ATC literature settles on.
	defaultLookaheadK = 2.0
	// lockDominance makes a date-locked commitment outweigh any combination of
	// cost of delay, tier and priority, which is what FR-011's "dominate" means.
	// The weight terms are bounded (CoD <= 10, tier <= 4, priority <= 2), so 100
	// exceeds every achievable ratio between two unlocked initiatives.
	lockDominance = 100
	// weeksPerQuarter bounds the change-absorption cap (FR-026).
	weeksPerQuarter = 13
)

// The org WIP models a planner may choose between (D22 as amended 2026-08-20).
// The limit encodes a belief about multitasking that the schedule cannot confirm —
// Decision 4 removed the queue multiplier, which is the term that would have
// priced it — so the tool offers the readings rather than picking one.
const (
	// WipUnchosen is a state, not a model: the plan has not chosen. It schedules as
	// WipStrict so an existing plan's order does not move, and AC 1.1 still gets its
	// ranks and weeks, while the response says the choice is outstanding.
	WipUnchosen = "unchosen"
	// WipStrict counts every initiative against the drum's tracks. Textbook
	// drum-buffer-rope: protect the constraint absolutely, accept idle elsewhere.
	WipStrict = "strict"
	// WipDrumGated counts only initiatives that consume a drum pod, so work that
	// never touches the constraint is not held back by it.
	WipDrumGated = "drum-gated"
	// WipOff applies no org WIP limit. The per-pod cap, the change-absorption cap,
	// leads, pod tracks, calendars and dependencies all still apply: this switches
	// off one limit, not every limit. Also the model under which AC 4.2 is checkable.
	WipOff = "off"
)

// wipModels is the set offered, in the order the comparison is reported.
var wipModels = []string{WipStrict, WipDrumGated, WipOff}

// defaultLeadCapacity is how many initiatives one named lead can front at once
// (§10 Q3). A supplied role always wins, including a 0, which means that lead
// cannot take an initiative at all. Keys match leadKey().
var defaultLeadCapacity = map[string]int{"pm": 2, "eng": 2, "architect": 3, "pgm": 4}

// SchedulingParams is the plan-level scheduling policy (§7). Every field is
// optional: a plan supplying none of them still schedules (FR-002).
type SchedulingParams struct {
	PeriodStart              string         `json:"periodStart,omitempty"`              // ISO date mapping to week 0
	MaxConcurrentInitiatives int            `json:"maxConcurrentInitiatives,omitempty"` // org WIP limit; 0 derives from the drum
	MaxInitiativesPerPod     int            `json:"maxInitiativesPerPod,omitempty"`     // per-pod concurrency cap; 0 = uncapped
	KitGate                  float64        `json:"kitGate,omitempty"`                  // minimum full-kit readiness to release
	TargetUtilization        float64        `json:"targetUtilization,omitempty"`        // drum stagger ceiling; 0 = no stagger
	BufferPct                *float64       `json:"bufferPct,omitempty"`                // absent = 0.25; explicit 0 commits on the raw finish
	FeedingBufferPct         *float64       `json:"feedingBufferPct,omitempty"`         // reserved for feeding paths
	MaxStartsPerQuarter      int            `json:"maxStartsPerQuarter,omitempty"`      // change-absorption cap; 0 = uncapped
	LeadCapacity             map[string]int `json:"leadCapacity,omitempty"`             // role -> concurrent initiatives
	AllowTransfers           bool           `json:"allowTransfers,omitempty"`           // reserved for capacity transfer
	TransferRampWeeks        int            `json:"transferRampWeeks,omitempty"`        // reserved for capacity transfer
	LookaheadK               float64        `json:"lookaheadK,omitempty"`               // tardiness-index slack discount
	WipModel                 string         `json:"wipModel,omitempty"`                 // strict | drum-gated | off; absent = unchosen
}

// wipModel is the model in force, treating anything unrecognised as unchosen: a
// model this build does not implement is not a licence to invent one.
func (sp SchedulingParams) wipModel() string {
	switch sp.WipModel {
	case WipStrict, WipDrumGated, WipOff:
		return sp.WipModel
	}
	return WipUnchosen
}

// effectiveWipModel is the rule actually applied. Unchosen schedules as strict.
func (sp SchedulingParams) effectiveWipModel() string {
	if m := sp.wipModel(); m != WipUnchosen {
		return m
	}
	return WipStrict
}

// bufferFraction is the flat chain percentage from Decision 20.
func (sp SchedulingParams) bufferFraction() float64 {
	if sp.BufferPct == nil {
		return defaultBufferPct
	}
	if *sp.BufferPct < 0 {
		return 0
	}
	return *sp.BufferPct
}

func (sp SchedulingParams) lookahead() float64 {
	if sp.LookaheadK <= 0 {
		return defaultLookaheadK
	}
	return sp.LookaheadK
}

// leadCap is the concurrent-initiative capacity of one lead role. A role present
// in the supplied map wins even at 0; an absent role takes the default.
func (sp SchedulingParams) leadCap(role string) int {
	if n, ok := sp.LeadCapacity[role]; ok {
		return n
	}
	if n, ok := defaultLeadCapacity[role]; ok {
		return n
	}
	return 1
}

// bufferWeeksFor sizes an initiative's buffer. Decision 20 asks for exactly one
// function here: if per-slice uncertainty ever arrives, square-root-of-sum-of-
// squares replaces this body and no caller changes.
func bufferWeeksFor(chainWeeks int, sp SchedulingParams) int {
	if chainWeeks <= 0 {
		return 0
	}
	return int(math.Ceil(float64(chainWeeks) * sp.bufferFraction()))
}

// handoffWeeks is the cross-site handoff allowance between two pods (FR-006).
//
// It is 0 for now, and deliberately so: §10 Q1 deferred where working-hours
// overlap for a plan roster comes from, and its answer is the plan-level site
// table in spec 003, not a pair of same-site/cross-site constants invented here.
// Inventing them would be the unsourced capacity claim Decision 22 argues
// against. The seam stays so that the allowance, and the "handoff" binding
// constraint that goes with it, arrive as a change to this body.
func handoffWeeks(_, _ Team) int { return 0 }

// WorkSlice is one initiative's work at one pod, placed in time (§7).
type WorkSlice struct {
	Initiative        string  `json:"initiative"`
	Pod               string  `json:"pod"`
	RemainingWeeks    float64 `json:"remainingWeeks"` // after carryover
	StartWeek         int     `json:"startWeek"`
	FinishWeek        int     `json:"finishWeek"` // exclusive
	WaitWeeks         float64 `json:"waitWeeks"`  // ready to started
	BindingConstraint string  `json:"bindingConstraint,omitempty"`
	Estimated         bool    `json:"estimated"`
	// The in-plan, cycle-broken predecessors this slice waits on — the arrows
	// the timeline draws (FR-037) and the upstream names FR-042 requires.
	DependsOn []string `json:"dependsOn,omitempty"`
	// The last week this slice can begin without moving its initiative's
	// commit date, and the weeks it may therefore wait (FR-041). Zero slack
	// marks the critical chain (AC 9.3). Both are relative to the schedule as
	// computed — a slice that waits for capacity now has that waiting priced
	// in, so its slack is what is left after the schedule's own delays.
	LatestStartWeek int `json:"latestStartWeek"`
	SlackWeeks      int `json:"slackWeeks"`
}

// PodWeek is one pod's load in one week.
type PodWeek struct {
	Week        int      `json:"week"`
	Busy        int      `json:"busy"` // tracks occupied
	Tracks      int      `json:"tracks"`
	Utilization float64  `json:"utilization"`
	Initiatives []string `json:"initiatives,omitempty"`
}

// PodSchedule is one pod's whole period: its weekly load and its queue in
// scheduled start order (FR-004, AC 4.3).
type PodSchedule struct {
	Pod    string      `json:"pod"`
	Tracks int         `json:"tracks"`
	Weeks  []PodWeek   `json:"weeks"`
	Slices []WorkSlice `json:"slices"`
}

// RankingTerms is the ranking formula's named terms for one initiative, so the
// UI can show why it sits where it sits rather than asserting a number (FR-021).
type RankingTerms struct {
	Weight          float64 `json:"weight"`          // cost of delay x tier x stated priority
	ConstraintWeeks float64 `json:"constraintWeeks"` // its consumption of the drum pods
	SlackWeeks      float64 `json:"slackWeeks"`      // target minus chain, at ranking time
	Index           float64 `json:"index"`           // the dispatch index itself
	Rule            string  `json:"rule"`            // the rule that won
}

// ScheduledInitiative is one initiative's place in the order (§7).
type ScheduledInitiative struct {
	Name              string       `json:"name"`
	ProposedRank      int          `json:"proposedRank"`
	StatedRank        int          `json:"statedRank,omitempty"`
	StartWeek         int          `json:"startWeek"`
	RawFinishWeek     int          `json:"rawFinishWeek"`
	CommitWeek        int          `json:"commitWeek"`
	BufferWeeks       int          `json:"bufferWeeks"`
	TargetWeek        *int         `json:"targetWeek,omitempty"`
	Verdict           string       `json:"verdict"`
	WeeksLate         int          `json:"weeksLate,omitempty"`
	BindingConstraint string       `json:"bindingConstraint,omitempty"`
	PriorityLocked    bool         `json:"priorityLocked,omitempty"`
	DateLocked        bool         `json:"dateLocked,omitempty"`
	Provisional       bool         `json:"provisional,omitempty"`     // rests on unestimated work (AC 2.5)
	UnestimatedPods   []string     `json:"unestimatedPods,omitempty"` // which pods left it blank
	Assumptions       []string     `json:"assumptions,omitempty"`
	RankingTerms      RankingTerms `json:"rankingTerms"`
	Slices            []WorkSlice  `json:"slices"`
}

// RankDeviation reports one initiative whose proposed rank differs from the
// planner's stated rank, with the reason (FR-012). The portfolio-level price of
// keeping the stated order is the gap between the two objective scores on
// Schedule; per-remedy pricing belongs to the remedies endpoint.
type RankDeviation struct {
	Initiative   string `json:"initiative"`
	StatedRank   int    `json:"statedRank"`
	ProposedRank int    `json:"proposedRank"`
	Reason       string `json:"reason"`
}

// WipLimit is the org WIP limit in force, and where it came from. Decision 22
// requires a derived value to be labelled as derived and to name its pod,
// because the limit moving when the roster moves is correct but surprising.
type WipLimit struct {
	Value   int    `json:"value"`
	Derived bool   `json:"derived"`
	FromPod string `json:"fromPod,omitempty"`
	Model   string `json:"model"` // which rule the limit follows, or "unchosen"
}

// WipModelOutcome is what one model would cost for this plan. Reported for all
// three whichever is in force, because the comparison describes the plan rather
// than the current choice — and static help text cannot say what a model costs here.
type WipModelOutcome struct {
	Model             string  `json:"model"`
	Limit             int     `json:"limit"`
	LastCommitWeek    int     `json:"lastCommitWeek"`
	DatesMissed       int     `json:"datesMissed"`
	Late              int     `json:"late"`
	Infeasible        int     `json:"infeasible"`
	PodsIdleAllPeriod int     `json:"podsIdleAllPeriod"`
	Objective         float64 `json:"objective"`
}

// RuleScore is one dispatch rule's objective, so the UI can say "best of 5".
type RuleScore struct {
	Rule      string  `json:"rule"`
	Objective float64 `json:"objective"`
}

// Schedule is the whole computed order (§7).
type Schedule struct {
	Initiatives               []ScheduledInitiative `json:"initiatives"`
	PodWeeks                  []PodSchedule         `json:"podWeeks"`
	DrumPods                  []string              `json:"drumPods"`
	WipLimit                  WipLimit              `json:"wipLimit"`
	Rule                      string                `json:"rule"`
	RulesTried                []RuleScore           `json:"rulesTried"`
	ObjectiveScore            float64               `json:"objectiveScore"`
	StatedOrderObjectiveScore float64               `json:"statedOrderObjectiveScore"`
	WipModels                 []WipModelOutcome     `json:"wipModels,omitempty"`
	Reconciliation            []RankDeviation       `json:"reconciliation,omitempty"`
	Conflicts                 []Conflict            `json:"conflicts,omitempty"`
	Assumptions               []string              `json:"assumptions,omitempty"`
	Warnings                  []string              `json:"warnings,omitempty"`
	HorizonWeeks              int                   `json:"horizonWeeks"`
	PeriodStart               string                `json:"periodStart,omitempty"`
}

// schedInput is one initiative digested once, before any rule runs: its pods in
// dependency order, whole-week durations, delay weight and dates as week
// numbers. Rules and packing read it; nothing mutates it.
type schedInput struct {
	idx         int
	init        Initiative
	order       []string // in-path pods, dependencies first
	deps        map[string][]string
	durations   map[string]int
	drumWeeks   float64 // consumption of the drum pods
	totalWeeks  float64
	chainAlone  int // critical chain at unlimited capacity
	weight      float64
	targetWeek  *int
	earliest    int
	unestimated []string
	unknownPods []string
	assumptions []string
}

// ComputeSchedule builds the execution order for a plan. It is a pure function:
// same inputs, identical output, field for field (AC 1.4).
//
// It also reports what each WIP model would cost for this plan (D22 as amended),
// which is why it computes four schedules: the answer, and one per model for the
// comparison. The comparison is of the planner's own plan on purpose — static help
// text can describe a model but cannot say what choosing it costs here.
func ComputeSchedule(teams []Team, inits []Initiative, params Params, sp SchedulingParams) *Schedule {
	sched := computeOne(teams, inits, params, sp)
	sched.WipModels = compareWipModels(teams, inits, params, sp)
	return sched
}

// compareWipModels summarises each offered model. It reports the same three
// outcomes whichever model is in force, because the comparison describes the plan
// rather than the current choice.
func compareWipModels(teams []Team, inits []Initiative, params Params, sp SchedulingParams) []WipModelOutcome {
	horizon := int(math.Ceil(params.WithDefaults().HorizonWeeks))
	out := make([]WipModelOutcome, 0, len(wipModels))
	for _, model := range wipModels {
		alt := sp
		alt.WipModel = model
		run := computeOne(teams, inits, params, alt)

		o := WipModelOutcome{Model: model, Limit: run.WipLimit.Value, Objective: run.ObjectiveScore}
		for _, si := range run.Initiatives {
			if si.CommitWeek > o.LastCommitWeek {
				o.LastCommitWeek = si.CommitWeek
			}
			switch si.Verdict {
			case verdictLate:
				o.Late++
			case verdictInfeasible:
				o.Infeasible++
			}
		}
		o.DatesMissed = o.Late + o.Infeasible
		for _, ps := range run.PodWeeks {
			busy := false
			for w := 0; w < horizon && w < len(ps.Weeks); w++ {
				if ps.Weeks[w].Busy > 0 {
					busy = true
					break
				}
			}
			if !busy {
				o.PodsIdleAllPeriod++
			}
		}
		out = append(out, o)
	}
	return out
}

// computeOne is one schedule, without the model comparison — the comparison calls
// it per model, so it cannot be the thing that builds the comparison.
func computeOne(teams []Team, inits []Initiative, params Params, sp SchedulingParams) *Schedule {
	params = params.WithDefaults()
	horizon := int(math.Ceil(params.HorizonWeeks))

	byName := map[string]Team{}
	tracks := map[string]int{}
	for _, t := range teams {
		byName[t.Name] = t
		tracks[t.Name] = t.EffectiveTracks()
	}

	// Two passes, because the drum has to be chosen from the same residual work the
	// ranking uses: durations first, then the drum they imply, then the per-initiative
	// drum consumption. Utilization is deliberately not the source here — it reports
	// full demand for the existing flat-rho views, whose numbers must not move
	// (AC 1.1), so a nearly-finished carryover would otherwise crown the wrong pod.
	prepared := make([]*schedInput, 0, len(inits))
	for i, it := range inits {
		prepared = append(prepared, prepareInitiative(i, it, tracks, params, sp))
	}
	drumPods, drum := drumsOf(residualLoads(prepared, tracks, params))
	wip := deriveWipLimit(sp, tracks, drum)
	for _, in := range prepared {
		in.setDrumWeeks(drumPods)
	}

	pBar := meanProcessing(prepared)

	var best *runResult
	var bestRule string
	statedObjective := 0.0
	var tried []RuleScore
	for _, rule := range dispatchRules {
		run := generate(prepared, rankOrder(rule, prepared, sp, pBar), byName, tracks, sp, wip, horizon, pBar)
		tried = append(tried, RuleScore{Rule: rule, Objective: run.objective})
		if rule == ruleStatedPriority {
			statedObjective = run.objective
		}
		// Strictly better only, so a tie keeps the earlier rule and the winner
		// never depends on map or float ordering.
		if best == nil || run.objective < best.objective-1e-9 {
			best, bestRule = run, rule
		}
	}

	sched := &Schedule{
		Initiatives:               best.initiatives,
		PodWeeks:                  best.pods,
		DrumPods:                  drumPods,
		WipLimit:                  wip,
		Rule:                      bestRule,
		RulesTried:                tried,
		ObjectiveScore:            best.objective,
		StatedOrderObjectiveScore: statedObjective,
		HorizonWeeks:              horizon,
		PeriodStart:               sp.PeriodStart,
	}
	for i := range sched.Initiatives {
		sched.Initiatives[i].RankingTerms.Rule = bestRule
	}
	sched.Reconciliation = reconcile(sched.Initiatives, bestRule)
	sched.Conflicts = conflictingCommitments(sched.Initiatives)
	annotateSliceSlack(sched.Initiatives)
	// PodWeeks was built during the run, before the annotation: the pod view
	// reads those copies, so the slack pair has to reach them too.
	propagateSliceSlack(sched.Initiatives, sched.PodWeeks)
	sched.Assumptions, sched.Warnings = notices(prepared)
	// From the winning run, not from a fresh walk of the plan: which edge closes a
	// cycle depends on the traversal, so a sheet-order detector would name an edge
	// this schedule did not break, and blame the wrong initiative for skipping it.
	sched.Assumptions = append(sched.Assumptions, best.assumptions...)
	return sched
}

// drumsOf picks the constraint pods the release rule staggers against: every pod
// at or over capacity, or the hottest one when none is. Pods with demand but no
// tracks are skipped — they are the unknown-pod case (AC X.2), not a drum, and
// deriving a WIP limit from a pod with zero tracks would floor it at zero.
func drumsOf(loads []PodLoad) (pods []string, drum string) {
	// Utilization sorts on rho alone, over a map, so two equally loaded pods can
	// arrive in either order. Pick the drum by value with the pod name as the
	// tiebreak instead of trusting that order, or the derived WIP limit and the
	// whole ranking would wander between identical runs (AC 1.4).
	hottest := -1.0
	for _, l := range loads {
		if l.Tracks <= 0 {
			continue
		}
		if l.Rho > hottest || (l.Rho == hottest && l.Team < drum) {
			hottest, drum = l.Rho, l.Team
		}
		if l.Rho >= 1 {
			pods = append(pods, l.Team)
		}
	}
	sort.Strings(pods)
	if len(pods) == 0 && drum != "" {
		pods = []string{drum}
	}
	return pods, drum
}

// deriveWipLimit implements Decision 22: an explicit value wins, otherwise the
// limit is the tracks at the drum pod, labelled as derived and naming the pod.
func deriveWipLimit(sp SchedulingParams, tracks map[string]int, drum string) WipLimit {
	model := sp.wipModel()
	if sp.effectiveWipModel() == WipOff {
		// No org limit at all. An explicit number is ignored on purpose: choosing "off"
		// and typing a number are contradictory instructions, and the model is the one
		// the planner picked most recently and most deliberately.
		return WipLimit{Model: model}
	}
	if sp.MaxConcurrentInitiatives > 0 {
		return WipLimit{Value: sp.MaxConcurrentInitiatives, Model: model}
	}
	n := tracks[drum]
	if n < 1 {
		n = 1 // a roster with no capacity at all still has to make progress
	}
	return WipLimit{Value: n, Derived: true, FromPod: drum, Model: model}
}

func prepareInitiative(idx int, it Initiative, tracks map[string]int, params Params, sp SchedulingParams) *schedInput {
	in := &schedInput{idx: idx, init: it, durations: map[string]int{}, weight: initiativeWeight(it)}

	inPath := map[string]bool{}
	for pod, w := range it.Work {
		if w.InPath {
			inPath[pod] = true
		}
	}
	in.order, in.deps, in.assumptions = podOrder(it.Work, inPath)

	for _, pod := range in.order {
		w := it.Work[pod]
		in.durations[pod] = sliceWeeks(w, it, params.CapacityLoss)
		// Rank on the capacity still to be consumed, not the original estimate: an
		// initiative that is 80% done occupies two more weeks of the drum, not ten,
		// and ranking it as though it were whole starves work that has more left.
		in.totalWeeks += float64(in.durations[pod])
		if !w.Estimated || w.Weeks <= 0 {
			in.unestimated = append(in.unestimated, pod)
		}
		if tracks[pod] <= 0 {
			in.unknownPods = append(in.unknownPods, pod)
		}
	}
	in.chainAlone = chainLength(in)
	in.targetWeek = weekOf(sp.PeriodStart, it.TargetDate)
	if w := weekOf(sp.PeriodStart, it.EarliestStart); w != nil && *w > 0 {
		in.earliest = *w
	}
	return in
}

// setDrumWeeks records how much of the drum this initiative consumes, which is the
// processing time the ranking index divides by (Decision 2). Separate from
// prepareInitiative because the drum is itself derived from the durations.
func (in *schedInput) setDrumWeeks(drumPods []string) {
	isDrum := map[string]bool{}
	for _, d := range drumPods {
		isDrum[d] = true
	}
	in.drumWeeks = 0
	for _, pod := range in.order {
		if isDrum[pod] {
			in.drumWeeks += float64(in.durations[pod])
		}
	}
}

// residualLoads is per-pod demand and utilization over the horizon, counting only
// the work that is actually left to do. Same shape as Utilization so drumsOf can
// read either, but computed from the scheduler's residual durations.
func residualLoads(ins []*schedInput, tracks map[string]int, params Params) []PodLoad {
	demand := map[string]float64{}
	for _, in := range ins {
		for _, pod := range in.order {
			demand[pod] += float64(in.durations[pod])
		}
	}
	names := make([]string, 0, len(demand))
	for pod := range demand {
		names = append(names, pod)
	}
	sort.Strings(names)

	out := make([]PodLoad, 0, len(names))
	for _, pod := range names {
		tr := tracks[pod]
		capw := float64(tr) * params.HorizonWeeks
		pl := PodLoad{Team: pod, DemandWeeks: demand[pod], Tracks: tr, CapacityWeeks: capw}
		switch {
		case capw > 0:
			pl.Rho = demand[pod] / capw
		case demand[pod] > 0:
			pl.Rho = InfiniteRho
		}
		pl.Constraint = pl.Rho >= 1
		out = append(out, pl)
	}
	return out
}

// podOrder returns the in-path pods with dependencies first. A dependency cycle
// is broken at the edge that closes it, and named as an assumption rather than
// silently dropped (AC X.1). Iteration is over sorted names so the order — and
// therefore which edge gets broken — is stable.
func podOrder(work map[string]TeamWork, inPath map[string]bool) ([]string, map[string][]string, []string) {
	names := make([]string, 0, len(inPath))
	for pod := range inPath {
		names = append(names, pod)
	}
	sort.Strings(names)

	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	deps := map[string][]string{}
	var order, broken []string

	var visit func(pod string)
	visit = func(pod string) {
		if state[pod] == done {
			return
		}
		state[pod] = visiting
		upstream := append([]string(nil), work[pod].DependsOn...)
		sort.Strings(upstream)
		for _, d := range upstream {
			if !inPath[d] || d == pod {
				continue
			}
			if state[d] == visiting {
				broken = append(broken, "broke dependency cycle at "+d+" -> "+pod)
				continue
			}
			visit(d)
			deps[pod] = append(deps[pod], d)
		}
		state[pod] = done
		order = append(order, pod)
	}
	for _, pod := range names {
		visit(pod)
	}
	return order, deps, broken
}

// sliceWeeks is a slice's duration in whole weeks: the estimate less any
// carryover already done (AC X.4), inflated by the capacity lost to PTO, ramp
// and attrition — the one inflation Decision 4 keeps inside the schedule.
// Unestimated work contributes 0, so the initiative is scheduled on what it does
// have rather than being held out of the order entirely (AC 2.5).
func sliceWeeks(w TeamWork, it Initiative, loss float64) int {
	if !w.Estimated || w.Weeks <= 0 {
		return 0
	}
	rem := w.Weeks
	if it.InFlight {
		rem *= 1 - clampFrac(it.ProgressPct)
	}
	if rem <= 0 {
		return 0
	}
	if loss > 0 && loss < 1 {
		rem /= 1 - loss
	}
	return int(math.Ceil(rem))
}

// chainLength is the initiative's critical chain at unlimited capacity: the
// longest path through its own pod dependencies. It is the denominator for
// structural infeasibility (Decision 12) and for slack in the ranking index.
func chainLength(in *schedInput) int {
	finish := map[string]int{}
	longest := 0
	for _, pod := range in.order { // already in dependency order
		start := 0
		for _, d := range in.deps[pod] {
			if finish[d] > start {
				start = finish[d]
			}
		}
		finish[pod] = start + in.durations[pod]
		if finish[pod] > longest {
			longest = finish[pod]
		}
	}
	return longest
}

// initiativeWeight is the delay weight of FR-011: cost of delay scaled by
// requester tier and by the planner's stated priority. Each term is reported
// separately in RankingTerms, because a single opaque weighted sum cannot be
// explained line by line and FR-021 requires that it can.
func initiativeWeight(it Initiative) float64 {
	cod := it.CostOfDelayPerWeek
	if cod <= 0 {
		cod = 1 // unscored initiatives compete on duration and dates alone
	}
	return cod * tierWeight(it.Tier) * priorityWeight(it.StatedPriority)
}

// tierWeight follows the tier semantics the app already uses (GAME-SPEC §9):
// T1 contractual is the most expensive to miss, T4 aspirational the least.
func tierWeight(tier int) float64 {
	switch tier {
	case 1:
		return 4
	case 2:
		return 3
	case 3:
		return 2
	case 4:
		return 1
	}
	return 1
}

// priorityWeight turns a stated rank into a bounded multiplier: priority 1 is
// worth twice an unranked initiative and the value decays as the rank worsens.
// Bounded on purpose — priority shapes the order without determining it
// (Decision 3); priorityLocked is the tool for determining it.
func priorityWeight(p int) float64 {
	if p <= 0 {
		return 1
	}
	return 1 + 1/float64(p)
}

// atcIndex is Apparent Tardiness Cost (Decision 2): delay weight per unit of
// constraint-pod time, discounted exponentially by the slack left to the date.
// Processing time is consumption of the drum rather than total weeks, which is
// value per constraint week. With no date the discount is 1 and the index
// degenerates exactly to WSJF, as Decision 2 requires.
func atcIndex(in *schedInput, pBar, k float64) (index, slack float64) {
	p := in.drumWeeks
	if p <= 0 {
		p = in.totalWeeks
	}
	if p <= 0 {
		p = 1
	}
	base := in.weight / p
	if in.targetWeek == nil {
		return base, 0
	}
	slack = float64(*in.targetWeek - in.chainAlone)
	if slack < 0 {
		slack = 0
	}
	if pBar <= 0 {
		pBar = p
	}
	return base * math.Exp(-slack/(k*pBar)), slack
}

// rankOrder is the release order under one dispatch rule. Scores are computed
// once, at the period start, so the ranking is a formula a planner can
// reproduce; every rule then hands the same packing pass a different sequence.
//
// Priority locks are applied last, as positions rather than scores: a locked
// initiative is pinned to its stated rank so that its proposed rank equals its
// stated rank relative to all others, which is what AC 3.1 asks for.
func rankOrder(rule string, ins []*schedInput, sp SchedulingParams, pBar float64) []*schedInput {
	k := sp.lookahead()

	score := func(in *schedInput) float64 {
		idx, slack := atcIndex(in, pBar, k)
		switch rule {
		case ruleTardinessCost:
			return idx
		case "minimum-slack":
			if in.targetWeek == nil {
				return -math.MaxFloat32 // undated work has no slack to be short of
			}
			return -slack
		case "value-per-constraint-week":
			p := in.drumWeeks
			if p <= 0 {
				p = in.totalWeeks
			}
			if p <= 0 {
				p = 1
			}
			return in.weight / p
		case "constraint-first":
			return in.drumWeeks
		case ruleStatedPriority:
			if in.init.StatedPriority <= 0 {
				return -math.MaxFloat32 // unranked initiatives go last, in sheet order
			}
			return -float64(in.init.StatedPriority)
		}
		return idx
	}

	var locked, free []*schedInput
	for _, in := range ins {
		if in.init.PriorityLocked && in.init.StatedPriority > 0 {
			locked = append(locked, in)
		} else {
			free = append(free, in)
		}
	}
	// Sheet order is the last tiebreak everywhere, so two initiatives the rule
	// cannot separate keep the order the planner typed them in.
	sort.SliceStable(free, func(a, b int) bool {
		sa, sb := score(free[a]), score(free[b])
		if sa != sb {
			return sa > sb
		}
		if free[a].init.StatedPriority != free[b].init.StatedPriority {
			return statedBefore(free[a].init.StatedPriority, free[b].init.StatedPriority)
		}
		return free[a].idx < free[b].idx
	})
	sort.SliceStable(locked, func(a, b int) bool {
		if locked[a].init.StatedPriority != locked[b].init.StatedPriority {
			return locked[a].init.StatedPriority < locked[b].init.StatedPriority
		}
		return locked[a].idx < locked[b].idx
	})

	out := make([]*schedInput, len(ins))
	for _, in := range locked {
		out[freeSlotNear(out, in.init.StatedPriority-1)] = in
	}
	next := 0
	for _, in := range free {
		for out[next] != nil {
			next++
		}
		out[next] = in
	}
	return out
}

// statedBefore orders stated priorities with 1 highest and 0 meaning unranked,
// which sorts last rather than first.
func statedBefore(a, b int) bool {
	if a == 0 {
		return false
	}
	if b == 0 {
		return true
	}
	return a < b
}

// freeSlotNear finds the empty rank position closest to want, preferring later
// positions. Two initiatives locked to the same stated priority is legal (ties
// are allowed, §10 Q8), so one of them has to take the next slot.
func freeSlotNear(out []*schedInput, want int) int {
	if want < 0 {
		want = 0
	}
	if want >= len(out) {
		want = len(out) - 1
	}
	for i := want; i < len(out); i++ {
		if out[i] == nil {
			return i
		}
	}
	for i := want - 1; i >= 0; i-- {
		if out[i] == nil {
			return i
		}
	}
	return 0
}

// meanProcessing is the portfolio's average consumption of the drum, the
// denominator the tardiness index discounts slack against. Both the ranking and
// the terms reported alongside it have to use the same one (FR-021).
func meanProcessing(ins []*schedInput) float64 {
	if len(ins) == 0 {
		return 0
	}
	total := 0.0
	for _, in := range ins {
		p := in.drumWeeks
		if p <= 0 {
			p = in.totalWeeks
		}
		total += p
	}
	return total / float64(len(ins))
}

// runResult is one dispatch rule's completed schedule.
type runResult struct {
	initiatives []ScheduledInitiative
	pods        []PodSchedule
	objective   float64
	assumptions []string // precedence edges this run had to break
}

// podCalendar tracks one pod's occupancy week by week.
type podCalendar struct {
	tracks int
	busy   []int
	byWeek []map[string]bool // week -> initiatives occupying the pod
}

// generate is the serial schedule-generation scheme: take initiatives in the
// rule's order, gate each one's release (Decision 5), then place its slices as
// early as capacity allows. Releases are what get held back; released work runs.
func generate(all []*schedInput, order []*schedInput, teams map[string]Team, tracks map[string]int,
	sp SchedulingParams, wip WipLimit, horizon int, pBar float64) *runResult {

	maxWeek := horizon
	for _, in := range all {
		maxWeek += in.chainAlone + 1
	}

	cal := map[string]*podCalendar{}
	calendarFor := func(pod string) *podCalendar {
		c := cal[pod]
		if c == nil {
			c = &podCalendar{tracks: tracks[pod]}
			cal[pod] = c
		}
		return c
	}

	var inFlight []int
	leadBusy := map[string][]int{}
	quarterStarts := map[int]int{}
	commitOf := map[string]int{}

	ranks := map[string]int{}
	for i, in := range order {
		if in != nil {
			ranks[in.init.Name] = i + 1
		}
	}

	seq, brokenPrecedence := releaseSequence(order)

	results := map[string]*ScheduledInitiative{}
	for _, in := range seq {
		rank := ranks[in.init.Name]
		release, reason := releaseFloor(in, sp, commitOf, horizon)

		var placed []WorkSlice
		var start, finish int
		for {
			placed, start, finish = planSlices(in, release, cal, teams, tracks, sp.MaxInitiativesPerPod)
			// Carryover is already running, so no release gate can push it later — but
			// it does occupy its slots, which the bookkeeping below records (AC X.4).
			if in.init.InFlight {
				break
			}
			gate, ok := releaseGates(in, sp, wip, start, finish, inFlight, leadBusy, quarterStarts)
			if ok || release > maxWeek {
				break
			}
			release, reason = release+1, gate
		}

		for _, s := range placed {
			c := calendarFor(s.Pod)
			if c.tracks <= 0 || s.FinishWeek <= s.StartWeek {
				continue // unknown pods consume nothing; zero-length slices occupy nothing
			}
			for w := s.StartWeek; w < s.FinishWeek; w++ {
				bumpInt(&c.busy, w, 1)
				markWeek(&c.byWeek, w, in.init.Name)
			}
		}
		if sp.effectiveWipModel() != WipDrumGated || in.drumWeeks > 0 {
			// Only work the limit applies to occupies a slot. Otherwise an initiative
			// exempt from the limit would still consume the capacity it is exempt from,
			// and drum-gated would throttle the drum work it exists to protect.
			for w := start; w < finish; w++ {
				bumpInt(&inFlight, w, 1)
			}
		}
		for role, name := range in.init.Leads {
			key := role + "|" + name
			slot := leadBusy[key]
			for w := start; w < finish; w++ {
				bumpInt(&slot, w, 1)
			}
			leadBusy[key] = slot
		}
		quarterStarts[start/weeksPerQuarter]++

		si := summarise(in, rank, start, finish, placed, reason, sp, pBar)
		commitOf[in.init.Name] = si.CommitWeek
		results[in.init.Name] = &si
	}

	// Emit initiatives in sheet order: the rank is a field, not a row position,
	// so the response is stable however the rules reorder the release sequence.
	out := make([]ScheduledInitiative, 0, len(all))
	weights := map[string]float64{}
	for _, in := range all {
		if si := results[in.init.Name]; si != nil {
			out = append(out, *si)
			weights[in.init.Name] = in.weight
		}
	}
	return &runResult{initiatives: out, pods: podSchedules(cal, tracks, out, horizon),
		objective: objectiveOf(out, weights), assumptions: brokenPrecedence}
}

// releaseSequence is the order releases are actually processed in, which is not
// the same thing as the rank order:
//
//   - carryover first, because it is already running at the period start and its
//     slots are taken before anything new can be released (AC X.4);
//   - then rank order, reordered so an initiative never precedes an initiative it
//     declares itself to be after. A predecessor may rank below its dependent, and
//     without this the dependent would be placed while commitOf held no entry for
//     the predecessor, so FR-007 would be skipped in silence rather than enforced.
//
// A cycle in afterInitiatives is broken at the edge that closes it, on the same
// reasoning as a pod cycle (AC X.1): scheduling something and saying so beats
// refusing to schedule at all. The broken edges come back with the sequence so the
// assumption describes the order that was actually returned.
func releaseSequence(order []*schedInput) ([]*schedInput, []string) {
	byName := map[string]*schedInput{}
	var ranked []*schedInput
	for _, in := range order {
		if in == nil {
			continue
		}
		byName[in.init.Name] = in
		ranked = append(ranked, in)
	}

	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var seq []*schedInput
	var brokenEdges []string
	var visit func(in *schedInput)
	visit = func(in *schedInput) {
		if state[in.init.Name] == done {
			return
		}
		state[in.init.Name] = visiting
		for _, pred := range in.init.AfterInitiatives {
			p := byName[pred]
			// An unknown predecessor is not in this plan, so there is nothing to wait
			// for; releaseFloor already ignores it for the same reason.
			if p == nil || p == in {
				continue
			}
			if state[pred] == visiting {
				// This edge closes a cycle. Breaking it is the only way to produce an
				// order at all, and it means this initiative will be released without
				// its predecessor's finish, which AC X.1 requires us to say out loud.
				brokenEdges = append(brokenEdges,
					"broke an initiative precedence cycle at "+pred+" -> "+in.init.Name+
						", so "+in.init.Name+" is ordered without waiting for it")
				continue
			}
			visit(p)
		}
		state[in.init.Name] = done
		seq = append(seq, in)
	}
	// Carryover is visited first so it claims its slots before any new release.
	for _, in := range ranked {
		if in.init.InFlight {
			visit(in)
		}
	}
	for _, in := range ranked {
		visit(in)
	}
	return seq, brokenEdges
}

// releaseFloor is the earliest week an initiative could be released, before
// contention: its earliest-start date, its predecessor initiatives' commits, and
// the full-kit readiness gate (FR-007).
func releaseFloor(in *schedInput, sp SchedulingParams, commitOf map[string]int, horizon int) (int, string) {
	week, reason := 0, ""
	// Carryover started before the period did. An earliest-start date, a predecessor
	// or a readiness gate can only describe work not yet begun, so reporting a later
	// floor for it would be a statement about the past (AC X.4).
	if in.init.InFlight {
		return week, reason
	}
	if in.earliest > week {
		week, reason = in.earliest, bindEarliestStart
	}
	for _, pred := range in.init.AfterInitiatives {
		if c, ok := commitOf[pred]; ok && c > week {
			week, reason = c, bindPredecessor
		}
	}
	// Readiness does not improve on its own inside the period — nothing here models
	// kit work — so an initiative below the gate cannot start within it. Carryover is
	// exempt: it is already running, and a readiness gate cannot un-start work.
	if sp.KitGate > 0 && !in.init.InFlight && in.init.KitPct < sp.KitGate && horizon > week {
		week, reason = horizon, bindKitGate
	}
	return week, reason
}

// releaseGates checks the concurrency limits over an initiative's whole span,
// not just its first week, so "no week has more than N in flight" holds as
// stated (AC 1.5). It returns the gate that refused, for the report.
func releaseGates(in *schedInput, sp SchedulingParams, wip WipLimit, start, finish int,
	inFlight []int, leadBusy map[string][]int, quarterStarts map[int]int) (string, bool) {

	// The change-absorption cap is not the WIP limit and is not switched off with it:
	// a planner who set it asked for it separately, and `off` removes one org limit
	// rather than every limit. It gets its own label so FR-008's "which limit delayed
	// this" is answerable.
	if sp.MaxStartsPerQuarter > 0 && quarterStarts[start/weeksPerQuarter] >= sp.MaxStartsPerQuarter {
		return bindStartsCap, false
	}
	// Under drum-gated, an initiative that consumes no drum time is not what the rope
	// is protecting, so the org limit has nothing to hold it back from; under strict
	// every initiative counts. Under off this still iterates — the limit is zero, so
	// the comparison never trips, rather than the check being skipped. Saying that
	// precisely matters: a future change that stops relying on the zero would
	// otherwise look safe.
	counts := sp.effectiveWipModel() != WipDrumGated || in.drumWeeks > 0
	if counts {
		for w := start; w < finish; w++ {
			if wip.Value > 0 && weekAt(inFlight, w) >= wip.Value {
				return bindWipLimit, false
			}
		}
	}
	for role, name := range in.init.Leads {
		limit := sp.leadCap(role)
		slot := leadBusy[role+"|"+name]
		for w := start; w < finish; w++ {
			if weekAt(slot, w) >= limit {
				return bindLead, false
			}
		}
	}
	return "", true
}

// planSlices places one initiative's slices without committing them, so a
// refused release can be retried a week later. Slices run as early as capacity
// allows once released (Decision 5).
func planSlices(in *schedInput, release int, cal map[string]*podCalendar,
	teams map[string]Team, tracks map[string]int, perPodCap int) ([]WorkSlice, int, int) {

	finishOf := map[string]int{}
	slices := make([]WorkSlice, 0, len(in.order))
	start, finish := release, release

	for i, pod := range in.order {
		ready, reason := release, ""
		for _, dep := range in.deps[pod] {
			at := finishOf[dep] + handoffWeeks(teams[dep], teams[pod])
			if at > ready {
				ready, reason = at, bindDependency
			}
		}
		d := in.durations[pod]
		begin := ready
		if d > 0 && tracks[pod] > 0 {
			if s, why := firstFreeWeek(cal[pod], tracks[pod], ready, d, in.init.Name, perPodCap); s > ready {
				begin, reason = s, why
			}
		}
		w := in.init.Work[pod]
		slices = append(slices, WorkSlice{
			Initiative: in.init.Name, Pod: pod, RemainingWeeks: float64(d),
			StartWeek: begin, FinishWeek: begin + d, WaitWeeks: float64(begin - ready),
			BindingConstraint: reason, Estimated: w.Estimated && w.Weeks > 0,
			DependsOn: append([]string(nil), in.deps[pod]...),
		})
		finishOf[pod] = begin + d
		if i == 0 || begin < start {
			start = begin
		}
		if begin+d > finish {
			finish = begin + d
		}
	}
	if len(slices) == 0 {
		return slices, release, release
	}
	return slices, start, finish
}

// firstFreeWeek is the earliest week at or after from where the pod can take the
// slice for d consecutive weeks, and which limit refused the earlier weeks. A
// slice never spans two tracks (§10 Q4), so the window has to be contiguous on
// one of them.
//
// Two separate limits apply, and they are reported separately because the
// remedies differ: no free track is pod capacity, answered by tracks or descope;
// too many initiatives at once is the pod's own WIP cap, answered by sequencing.
func firstFreeWeek(c *podCalendar, tracks, from, d int, initiative string, perPodCap int) (int, string) {
	// The limit that refused the slice's own ready week is the one worth naming;
	// whatever refuses a later candidate week is a consequence of that first wait.
	refused := ""
	for s := from; ; s++ {
		fits, why := true, ""
		for w := s; w < s+d; w++ {
			if c == nil {
				break
			}
			if weekAt(c.busy, w) >= tracks {
				fits, why = false, bindPodCapacity
				break
			}
			if perPodCap > 0 && podWeekInitiatives(c, w, initiative) >= perPodCap {
				fits, why = false, bindPodWipLimit
				break
			}
		}
		if fits {
			return s, refused
		}
		if refused == "" {
			refused = why
		}
	}
}

// podWeekInitiatives counts the distinct initiatives already occupying the pod in
// one week, not counting the one asking — a slice does not contend with itself.
func podWeekInitiatives(c *podCalendar, w int, exclude string) int {
	if w >= len(c.byWeek) || c.byWeek[w] == nil {
		return 0
	}
	n := 0
	for name := range c.byWeek[w] {
		if name != exclude {
			n++
		}
	}
	return n
}

// summarise turns a placed initiative into its reported row: the buffered commit
// week (Decision 9), the verdict, and the constraint that set its start.
func summarise(in *schedInput, rank, start, finish int, slices []WorkSlice, releaseReason string,
	sp SchedulingParams, pBar float64) ScheduledInitiative {
	si := ScheduledInitiative{
		Name: in.init.Name, ProposedRank: rank, StatedRank: in.init.StatedPriority,
		StartWeek: start, RawFinishWeek: finish,
		PriorityLocked: in.init.PriorityLocked, DateLocked: in.init.DateLocked,
		TargetWeek: in.targetWeek, Slices: slices,
		UnestimatedPods: in.unestimated, Provisional: len(in.unestimated) > 0,
		Assumptions: in.assumptions,
	}
	si.BufferWeeks = bufferWeeksFor(finish-start, sp)
	si.CommitWeek = finish + si.BufferWeeks

	// pBar, not in.drumWeeks: FR-021 wants the terms that produced the position, and
	// the ranking discounted slack against the portfolio mean.
	idx, slack := atcIndex(in, pBar, sp.lookahead())
	si.RankingTerms = RankingTerms{Weight: round1(in.weight), ConstraintWeeks: in.drumWeeks,
		SlackWeeks: slack, Index: round1(idx)}

	// The constraint that set the start: the release gate if one held it back,
	// otherwise whatever the first slice was waiting for.
	si.BindingConstraint = releaseReason
	if si.BindingConstraint == "" {
		for _, s := range slices {
			if s.StartWeek == start && s.BindingConstraint != "" {
				si.BindingConstraint = s.BindingConstraint
				break
			}
		}
	}
	if si.BindingConstraint == "" && len(slices) > 0 {
		// Nothing gated the release, so blame whatever delayed the earliest slice.
		earliest := slices[0]
		for _, s := range slices[1:] {
			if s.StartWeek < earliest.StartWeek {
				earliest = s
			}
		}
		si.BindingConstraint = earliest.BindingConstraint
	}
	si.Verdict, si.WeeksLate = verdictFor(in, si, sp)
	return si
}

// verdictFor reads the date verdict off the buffered commit week, never the raw
// finish (Decision 9), and separates a date no ordering could meet from one lost
// to contention (Decision 12) — they are different conversations.
func verdictFor(in *schedInput, si ScheduledInitiative, sp SchedulingParams) (string, int) {
	if len(in.unknownPods) > 0 {
		return verdictUnschedulable, 0
	}
	if in.targetWeek == nil {
		return verdictNoDate, 0
	}
	target := *in.targetWeek
	if si.CommitWeek <= target {
		return verdictOnTime, 0
	}
	late := si.CommitWeek - target
	// Alone, at unlimited capacity, from its earliest possible start.
	alone := in.earliest + in.chainAlone
	if alone+bufferWeeksFor(in.chainAlone, sp) > target {
		return verdictInfeasible, late
	}
	return verdictLate, late
}

// objectiveOf is the weighted-lateness score (FR-011). Date-locked commitments
// dominate, so no combination of the bounded weight terms can outbid one.
func objectiveOf(sis []ScheduledInitiative, weights map[string]float64) float64 {
	total := 0.0
	for _, si := range sis {
		if si.WeeksLate <= 0 {
			continue
		}
		w := weights[si.Name]
		if si.DateLocked {
			w *= lockDominance
		}
		total += w * float64(si.WeeksLate)
	}
	return round1(total)
}

// podSchedules turns the occupancy calendars into the per-pod weekly load and
// queue the heatmap and the pod view read (FR-004, AC 4.3).
func podSchedules(cal map[string]*podCalendar, tracks map[string]int, sis []ScheduledInitiative, horizon int) []PodSchedule {
	weeks := horizon
	byPod := map[string][]WorkSlice{}
	for _, si := range sis {
		for _, s := range si.Slices {
			byPod[s.Pod] = append(byPod[s.Pod], s)
			if s.FinishWeek > weeks {
				weeks = s.FinishWeek
			}
		}
	}
	names := make([]string, 0, len(byPod))
	for pod := range byPod {
		names = append(names, pod)
	}
	for pod := range cal {
		if _, ok := byPod[pod]; !ok {
			names = append(names, pod)
		}
	}
	sort.Strings(names)

	out := make([]PodSchedule, 0, len(names))
	for _, pod := range names {
		ps := PodSchedule{Pod: pod, Tracks: tracks[pod], Slices: byPod[pod]}
		sort.SliceStable(ps.Slices, func(a, b int) bool {
			if ps.Slices[a].StartWeek != ps.Slices[b].StartWeek {
				return ps.Slices[a].StartWeek < ps.Slices[b].StartWeek
			}
			return ps.Slices[a].Initiative < ps.Slices[b].Initiative
		})
		c := cal[pod]
		ps.Weeks = make([]PodWeek, weeks)
		for w := 0; w < weeks; w++ {
			pw := PodWeek{Week: w, Tracks: tracks[pod]}
			if c != nil {
				pw.Busy = weekAt(c.busy, w)
				if w < len(c.byWeek) {
					for name := range c.byWeek[w] {
						pw.Initiatives = append(pw.Initiatives, name)
					}
					sort.Strings(pw.Initiatives)
				}
			}
			if pw.Tracks > 0 {
				pw.Utilization = float64(pw.Busy) / float64(pw.Tracks)
			}
			ps.Weeks[w] = pw
		}
		out = append(out, ps)
	}
	return out
}

// reconcile reports every initiative the engine moved away from the planner's
// stated rank, with the reason (FR-012). Decision 3 makes this the primary
// artefact of the feature: reordering without an explanation reads as being
// ignored.
func reconcile(sis []ScheduledInitiative, rule string) []RankDeviation {
	// Name the rule that actually won. The earlier version described the tardiness
	// index no matter which rule produced the order, so on a plan where
	// constraint-first won, every row cited a rule that had not been used — an
	// explanation that is confidently wrong is worse than none.
	basis := ruleBasis(rule)
	var out []RankDeviation
	for _, si := range sis {
		if si.StatedRank <= 0 || si.StatedRank == si.ProposedRank {
			continue
		}
		reason := "outranked on " + basis
		switch {
		case si.PriorityLocked:
			reason = "priority locked, so its stated rank was pinned"
		case si.ProposedRank < si.StatedRank:
			reason = "promoted: better on " + basis + " than the initiatives it passed"
		}
		out = append(out, RankDeviation{Initiative: si.Name, StatedRank: si.StatedRank,
			ProposedRank: si.ProposedRank, Reason: reason})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].ProposedRank < out[b].ProposedRank })
	return out
}

// ruleBasis is the winning rule in words a planner can argue with, since the
// reconciliation report exists to be read rather than to be correct in private.
func ruleBasis(rule string) string {
	switch rule {
	case ruleTardinessCost:
		return "delay cost per week of drum time, discounted by slack"
	case "minimum-slack":
		return "how little slack is left to its date"
	case "value-per-constraint-week":
		return "value per week of drum time"
	case "constraint-first":
		return "how much of the drum it consumes"
	case ruleStatedPriority:
		return "the stated priority order"
	}
	return "the winning dispatch rule"
}

// notices collects the assumptions and warnings every schedule has to carry
// (FR-021): broken cycles, unestimated work, and pods that are not on the
// roster. The unknown-pod wording matches the warning Plan already shows.
func notices(ins []*schedInput) (assumptions, warnings []string) {
	seenA, seenW := map[string]bool{}, map[string]bool{}
	add := func(list *[]string, seen map[string]bool, msg string) {
		if !seen[msg] {
			seen[msg] = true
			*list = append(*list, msg)
		}
	}
	for _, in := range ins {
		for _, a := range in.assumptions {
			add(&assumptions, seenA, in.init.Name+": "+a)
		}
		if len(in.unestimated) > 0 {
			add(&assumptions, seenA, in.init.Name+": scheduled without an estimate for "+
				strings.Join(in.unestimated, ", ")+", so its dates are provisional")
		}
		if len(in.unknownPods) > 0 {
			add(&warnings, seenW, in.init.Name+" depends on "+strings.Join(in.unknownPods, ", ")+
				", which has no capacity on this roster, so it cannot be scheduled")
		}
	}
	return assumptions, warnings
}

// weekOf converts an ISO date to a week index from the period start, rounding up
// so a date part-way through a week means "finished by the end of that week".
// Either date missing or unparseable yields no week rather than week 0, so a
// blank cell never reads as a date at the period start.
func weekOf(periodStart, date string) *int {
	if strings.TrimSpace(periodStart) == "" || strings.TrimSpace(date) == "" {
		return nil
	}
	const iso = "2006-01-02"
	t0, err := time.Parse(iso, strings.TrimSpace(periodStart))
	if err != nil {
		return nil
	}
	t1, err := time.Parse(iso, strings.TrimSpace(date))
	if err != nil {
		return nil
	}
	days := t1.Sub(t0).Hours() / 24
	w := int(math.Ceil(days / 7))
	if w < 0 {
		w = 0
	}
	return &w
}

func weekAt(xs []int, i int) int {
	if i >= 0 && i < len(xs) {
		return xs[i]
	}
	return 0
}

func bumpInt(xs *[]int, i, delta int) {
	for len(*xs) <= i {
		*xs = append(*xs, 0)
	}
	(*xs)[i] += delta
}

func markWeek(weeks *[]map[string]bool, i int, name string) {
	for len(*weeks) <= i {
		*weeks = append(*weeks, nil)
	}
	if (*weeks)[i] == nil {
		(*weeks)[i] = map[string]bool{}
	}
	(*weeks)[i][name] = true
}
