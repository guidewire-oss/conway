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
	verdictBeyondHorizon = "beyond-horizon"
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
	bindFreeze        = "freeze" // a calendar window refused the start or finish (FR-018)
	bindStagger       = "drum stagger" // release held to keep drum load under the target (Decision 5)
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
	PeriodStart              string           `json:"periodStart,omitempty"`              // ISO date mapping to week 0
	MaxConcurrentInitiatives int              `json:"maxConcurrentInitiatives,omitempty"` // org WIP limit; 0 derives from the drum
	MaxInitiativesPerPod     int              `json:"maxInitiativesPerPod,omitempty"`     // per-pod concurrency cap; 0 = uncapped
	KitGate                  float64          `json:"kitGate,omitempty"`                  // minimum full-kit readiness to release
	TargetUtilization        float64          `json:"targetUtilization,omitempty"`        // drum stagger ceiling; 0 = no stagger
	BufferPct                *float64         `json:"bufferPct,omitempty"`                // absent = 0.25; explicit 0 commits on the raw finish
	FeedingBufferPct         *float64         `json:"feedingBufferPct,omitempty"`         // reserved for feeding paths
	MaxStartsPerQuarter      int              `json:"maxStartsPerQuarter,omitempty"`      // change-absorption cap; 0 = uncapped
	LeadCapacity             map[string]int   `json:"leadCapacity,omitempty"`             // role -> concurrent initiatives
	Calendars                []CalendarWindow `json:"calendars,omitempty"`                // FR-018 calendar constraints
	AllowTransfers           bool             `json:"allowTransfers,omitempty"`           // reserved for capacity transfer
	TransferRampWeeks        int              `json:"transferRampWeeks,omitempty"`        // reserved for capacity transfer
	LookaheadK               float64          `json:"lookaheadK,omitempty"`               // tardiness-index slack discount
	WipModel                 string           `json:"wipModel,omitempty"`                 // strict | drum-gated | off; absent = unchosen
	// EstimateModel (spec 006 Decision 2): effort divides each pod estimate
	// by the pod's lanes; wall-clock keeps the estimate as one lane's duration.
	// Absent means wall-clock so every existing plan keeps its dates.
	EstimateModel            string           `json:"estimateModel,omitempty"`            // effort | wall-clock; absent = wall-clock
	// AcceptedOrdering (spec 006 Decision 1): which ordering the planner put
	// in force — their own (stated/sheet) or the engine's accepted proposal.
	// Absent = stated; nothing about the maths changes, only which column the
	// Order view sorts by.
	AcceptedOrdering   string `json:"acceptedOrdering,omitempty"`   // stated | engine
	AcceptedOrderingAt int64  `json:"acceptedOrderingAt,omitempty"` // unix secs of the choice
	// SplitTaxWeeks (spec 007): org-level weeks of overhead charged per lane
	// split. Absent or 0 disables splitting — the slice takes all lanes or
	// waits, exactly as before.
	SplitTaxWeeks int `json:"splitTaxWeeks,omitempty"`
	// SplitMinWeeks (spec 007 amendment): only work at least this many single-
	// track-weeks is worth dividing — the coordination tax outweighs the gain
	// on small slices. 40 weeks over 2 tracks splits 20+20; 45 over 3 splits
	// 20+20+5; a 12-week slice under a 20-week threshold stays whole.
	SplitMinWeeks int `json:"splitMinWeeks,omitempty"`
}

const (
	EstimateEffort     = "effort"
	EstimateWallClock  = "wall-clock"
)

// estimateModel is the model in force; anything unrecognised falls back to
// wall-clock, the semantics every pre-spec-006 plan was scheduled under.
func (sp SchedulingParams) estimateModel() string {
	if sp.EstimateModel == EstimateEffort {
		return EstimateEffort
	}
	return EstimateWallClock
}

// acceptedOrdering reports which ordering the planner put in force (spec 006
// Decision 1). Only the explicit marker flips to the engine's order.
func (sp SchedulingParams) acceptedOrdering() string {
	if sp.AcceptedOrdering == "engine" {
		return "engine"
	}
	return "stated"
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

// LanePhase is one span of constant lane occupancy inside a split slice
// (spec 007). The phases tile the slice's span; their lane counts are what
// the heatmap counts and the timeline animates.
type LanePhase struct {
	FromWeek int `json:"fromWeek"`
	ToWeek   int `json:"toWeek"` // exclusive
	Lanes    int `json:"lanes"`
}

// WorkSlice is one initiative's work at one pod, placed in time (§7).
type WorkSlice struct {
	Initiative        string  `json:"initiative"`
	Pod               string  `json:"pod"`
	RemainingWeeks    float64 `json:"remainingWeeks"` // after carryover
	// LanesUsed is how many tracks this slice occupies per week (spec 006
	// Decision 2): 1 under wall-clock; under effort, the lanes the work needs
	// (capped at the pod's tracks) — a busy pod is honestly busy for everyone.
	LanesUsed int `json:"lanesUsed,omitempty"`
	// Phases (spec 007): the slice's lanes over time when splitting — it grows
	// as lanes free. Empty when the slice ran at a constant LanesUsed.
	Phases []LanePhase `json:"phases,omitempty"`
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
	// AC 4.2 (spec 004 Story 4): the flat-model comparison and the idle
	// attribution. MeanUtil is the mean of the weekly utilization over the
	// horizon; FlatRho is the residual demand over capacity the Network view
	// reports; Idle* bucket the track-weeks the pod did not work, by the cause
	// visible in the schedule.
	MeanUtil float64   `json:"meanUtil,omitempty"`
	FlatRho  float64   `json:"flatRho,omitempty"`
	Idle     IdleWeeks `json:"idle,omitempty"`
}

// IdleWeeks are track-weeks, not calendar weeks: a 3-track pod idle all week
// counts 3. HeldForRelease is the remainder after calendars and upstream
// waits — the WIP limit, the change-absorption cap, the kit gate, or plain
// starvation all land here, because the schedule cannot separate them without
// re-running placement counterfactually.
type IdleWeeks struct {
	Calendar       float64 `json:"calendar"`
	Upstream       float64 `json:"upstream"`
	HeldForRelease float64 `json:"heldForRelease"`
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
	Name          string `json:"name"`
	ProposedRank  int    `json:"proposedRank"`
	StatedRank    int    `json:"statedRank,omitempty"`
	StartWeek     int    `json:"startWeek"`
	RawFinishWeek int    `json:"rawFinishWeek"`
	CommitWeek    int    `json:"commitWeek"`
	BufferWeeks   int    `json:"bufferWeeks"`
	// FR-024 (spec 004 Story 2): plan-time fever point. TargetProgress is the
	// chain fraction elapsed by the target week; TargetBurn is the buffer
	// fraction consumed by then (0 when the date holds); BurnRatio zones it
	// exactly as the Observe fever chart zones (Decision: same thresholds, both
	// views must read alike). A zero buffer or no target leaves all three 0.
	TargetProgress    float64      `json:"targetProgress"`
	TargetBurn        float64      `json:"targetBurn"`
	BurnRatio         float64      `json:"burnRatio"`
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
	// Locked carries the initiative's pin state so the reconciliation view can
	// offer pin/unpin per row (spec 004 AC 1.1/1.2) without a second lookup.
	Locked bool `json:"priorityLocked"`
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

// ScheduleFit is the arithmetic behind Decision 28: how much work the plan asks
// for against how much the pods can absorb inside the period, and how many
// initiatives did not fit.
//
// It exists because a week number is the wrong answer to "why doesn't this fit".
// The plan that prompted the decision reported every initiative as "no-date" and
// committed one of 29 inside its horizon; what a planner needed to hear was that
// they were asking for 125% of the available capacity.
type ScheduleFit struct {
	// PodWeeksDemanded is every initiative's in-path work, whether or not it fitted.
	PodWeeksDemanded float64 `json:"podWeeksDemanded"`
	// TrackWeeksAvailable is tracks x horizon, less the capacity loss: what the
	// pods can actually absorb rather than their nameplate.
	TrackWeeksAvailable float64 `json:"trackWeeksAvailable"`
	// BeyondHorizon counts the initiatives that could not begin inside the period.
	BeyondHorizon int `json:"beyondHorizon"`
	// HeldBy counts which constraint refused each of them, most common first. The
	// demand figures above explain a plan that is over capacity; they explain
	// nothing about one held out by a WIP limit, and the lever differs.
	HeldBy []ConstraintCount `json:"heldBy,omitempty"`
}

// ConstraintCount is one bindingConstraint and how many initiatives it held out.
type ConstraintCount struct {
	Constraint string `json:"constraint"`
	Count      int    `json:"count"`
}

// appendCount tallies a constraint, keeping the list ordered by count so the
// caller can read the dominant one off the front without sorting again.
func appendCount(counts []ConstraintCount, name string) []ConstraintCount {
	for i := range counts {
		if counts[i].Constraint == name {
			counts[i].Count++
			for j := i; j > 0 && counts[j].Count > counts[j-1].Count; j-- {
				counts[j], counts[j-1] = counts[j-1], counts[j]
			}
			return counts
		}
	}
	return append(counts, ConstraintCount{Constraint: name, Count: 1})
}

// LoadPct is the demand as a percentage of what the period can absorb. Above 100
// the plan cannot fit however it is ordered, which is a different conversation
// from any individual date.
func (f ScheduleFit) LoadPct() float64 {
	if f.TrackWeeksAvailable <= 0 {
		return 0
	}
	return f.PodWeeksDemanded / f.TrackWeeksAvailable * 100
}

// Schedule is the whole computed order (§7).
type Schedule struct {
	Initiatives               []ScheduledInitiative `json:"initiatives"`
	Fit                       *ScheduleFit          `json:"fit"`
	PodWeeks                  []PodSchedule         `json:"podWeeks"`
	DrumPods                  []string              `json:"drumPods"`
	WipLimit                  WipLimit              `json:"wipLimit"`
	Rule                      string                `json:"rule"`
	RulesTried                []RuleScore           `json:"rulesTried"`
	ObjectiveScore            float64               `json:"objectiveScore"`
	StatedOrderObjectiveScore float64               `json:"statedOrderObjectiveScore"`
	WipModels                 []WipModelOutcome     `json:"wipModels,omitempty"`
	Reconciliation            []RankDeviation       `json:"reconciliation,omitempty"`
	// EngineRanks is the best dispatch rule's per-initiative rank when the
	// working order is the planner's (spec 006): the suggestion column. Empty
	// when the engine's order is in force — it IS the spine then.
	EngineRanks               map[string]int        `json:"engineRanks,omitempty"`
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
	drumSet     map[string]bool // which of its pods are drums (the stagger gate reads this)
	totalWeeks  float64
	chainAlone  int // critical chain at unlimited capacity
	weight      float64
	targetWeek  *int
	earliest    int
	unestimated []string
	unknownPods []string
	assumptions []string
}

// ScheduleOptions turns on work that is not needed to answer "what is the order".
type ScheduleOptions struct {
	// CompareWipModels adds the per-model comparison (D22 as amended). It costs one
	// extra full schedule per model, so it is opt-in: only the scheduling-assumptions
	// form reads it, and that form is a panel the planner opens.
	CompareWipModels bool
}

// ComputeSchedule builds the execution order for a plan, with the WIP-model
// comparison included. It is a pure function: same inputs, identical output, field
// for field (AC 1.4).
//
// The comparison stays on here because this is the signature every existing caller
// already has, including BaselineInputs.Recompute — and AC 7.1 requires a baseline
// to reproduce its stored schedule exactly, which a quietly narrower answer would
// break. Callers that do not need it use ComputeScheduleWith.
func ComputeSchedule(teams []Team, inits []Initiative, params Params, sp SchedulingParams) *Schedule {
	return ComputeScheduleWith(teams, inits, params, sp, ScheduleOptions{CompareWipModels: true})
}

// ComputeScheduleWith is ComputeSchedule with the optional work made explicit.
//
// The comparison is of the planner's own plan on purpose — static help text can
// describe a WIP model but cannot say what choosing it costs here. That is worth
// three extra schedules when someone is looking at it, and worth none when nobody
// is: on a plan loaded to its capacity, those three runs were three quarters of the
// cost of every /schedule request.
func ComputeScheduleWith(teams []Team, inits []Initiative, params Params, sp SchedulingParams,
	opts ScheduleOptions) *Schedule {
	sched := computeOne(teams, inits, params, sp)
	if opts.CompareWipModels {
		sched.WipModels = compareWipModels(teams, inits, params, sp)
	}
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
	rules := compileCalendars(parseCalendars(sp, horizon))

	var best *runResult
	var bestRule string
	var statedRun *runResult
	statedObjective := 0.0
	var tried []RuleScore
	for _, rule := range dispatchRules {
		run := generate(prepared, rankOrder(rule, prepared, sp, pBar), byName, tracks, sp, wip, horizon, pBar, rules, params.CapacityLoss)
		tried = append(tried, RuleScore{Rule: rule, Objective: run.objective})
		if rule == ruleStatedPriority {
			statedObjective = run.objective
			statedRun = run
		}
		// Strictly better only, so a tie keeps the earlier rule and the winner
		// never depends on map or float ordering.
		if best == nil || run.objective < best.objective-1e-9 {
			best, bestRule = run, rule
		}
	}
	// Spec 006 Decision 1: the planner's order is the working plan unless they
	// explicitly accepted an engine proposal. Every rule still runs — the
	// scores and the proposal ride along in RulesTried — but the schedule
	// handed back follows the stated order, priced like any other.
	//
	// The engine's best run keeps a per-initiative rank map, so the Order view
	// can show "the engine suggests N" beside the planner's spine without a
	// second schedule call.
	engineRanks := map[string]int{}
	if sp.acceptedOrdering() != "engine" && statedRun != nil {
		for _, si := range best.initiatives {
			engineRanks[si.Name] = si.ProposedRank
		}
		best, bestRule = statedRun, ruleStatedPriority
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
		EngineRanks:               engineRanks,
		HorizonWeeks:              horizon,
		PeriodStart:               sp.PeriodStart,
	}
	for i := range sched.Initiatives {
		sched.Initiatives[i].RankingTerms.Rule = bestRule
	}
	sched.Fit = fitOf(inits, sched.Initiatives, tracks, params, horizon)
	sched.Reconciliation = reconcile(sched.Initiatives, bestRule)
	sched.Conflicts = conflictingCommitments(sched.Initiatives)
	annotateSliceSlack(sched.Initiatives)
	// PodWeeks was built during the run, before the annotation: the pod view
	// reads those copies, so the slack pair has to reach them too.
	propagateSliceSlack(sched.Initiatives, sched.PodWeeks)
	// AC 4.2 (spec 004 Story 4): the aggregate-consistency sentence. Mean weekly
	// utilization beside the flat rho, and the idle track-weeks bucketed by the
	// cause the schedule can actually see.
	annotateIdle(sched, rules, byName, residualLoads(prepared, tracks, params), horizon)
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

	effort := sp.estimateModel() == EstimateEffort
	for _, pod := range in.order {
		w := it.Work[pod]
		// Lanes only divide under the effort model (spec 006 Decision 2);
		// wall-clock passes 1 so the arithmetic is untouched.
		laneDiv := 1
		if effort {
			laneDiv = tracks[pod]
		}
		in.durations[pod] = sliceWeeks(w, it, params.CapacityLoss, laneDiv)
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
	in.drumSet = isDrum
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
func sliceWeeks(w TeamWork, it Initiative, loss float64, lanes int) int {
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
	// Spec 006 Decision 2: under the effort model the estimate is total work,
	// shared across the pod's lanes — 60 pair-weeks on 3 pairs is ~20 weeks,
	// not 60. Wall-clock keeps the estimate as one lane's duration.
	if lanes > 1 {
		rem /= float64(lanes)
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
	sp SchedulingParams, wip WipLimit, horizon int, pBar float64, rules *calendarRules, capacityLoss float64) *runResult {

	maxWeek := horizon
	for _, in := range all {
		maxWeek += in.chainAlone + 1
	}
	// A freeze can push a finish past the ordinary bound by at most the number
	// of frozen weeks; without this headroom the retry loop can exhaust.
	for w := range rules.orgBlockStart {
		if w > maxWeek {
			maxWeek = w
		}
	}
	for w := range rules.orgBlockFinish {
		if w > maxWeek {
			maxWeek = w
		}
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
			placed, start, finish = planSlices(in, release, cal, teams, tracks, sp.MaxInitiativesPerPod, rules, sp, capacityLoss)
			// Carryover is already running, so no release gate can push it later — but
			// it does occupy its slots, which the bookkeeping below records (AC X.4).
			if in.init.InFlight {
				break
			}
			gate, ok := releaseGates(in, sp, wip, start, finish, inFlight, leadBusy, quarterStarts)
			if !ok {
				// A release gate refused: hold one week and name it.
				release, reason = release+1, gate
				continue
			}
			// Decision 28: the release search stops at the period. maxWeek is the
			// old bound -- horizon plus every initiative's chain -- which on an
			// oversubscribed plan let releases walk out hundreds of weeks looking
			// for room that does not exist inside the period.
			if release >= horizon || release > maxWeek {
				break
			}
			// Decision 5's stagger: with every other gate satisfied, hold the
			// release so planned load at the drum pods stays at or below
			// targetUtilization. Only initiatives that touch a drum are
			// staggered; everything else releases as before. The check reads
			// each drum slice's actual start week (the placement already found
			// where the work would run), against the occupancy the earlier
			// releases created there.
			if stagger, why := drumStagger(in, sp, cal, placed); stagger {
				release, reason = release+1, why
				continue
			}
			break
		}

		// Decision 28: nothing begins outside the period. Such an initiative occupies
		// no calendar and carries no start or commit week -- an invented week number a
		// decade out is worse than an honest absence -- so it skips the bookkeeping
		// below entirely, and the capacity it would have consumed stays available to
		// the initiatives that do fit.
		if !in.init.InFlight && start >= horizon {
			// When placement rather than a release gate pushed it out, the reason lives
			// on the slices -- pod-capacity, or a freeze. Taking it before they are
			// cleared is what lets ScheduleFit.HeldBy name those causes instead of
			// reporting an empty constraint (FR-021).
			held := reason
			if held == "" {
				for _, sl := range placed {
					if sl.BindingConstraint != "" {
						held = sl.BindingConstraint
						break
					}
				}
			}
			si := summarise(in, rank, start, finish, nil, held, sp, pBar)
			si.StartWeek, si.RawFinishWeek, si.CommitWeek, si.BufferWeeks = 0, 0, 0, 0
			si.WeeksLate = 0
			si.Verdict = verdictBeyondHorizon
			// A successor cannot start before a predecessor that never starts. The
			// bookkeeping below is skipped, so releaseFloor would find no commit
			// recorded for this one and let dependents run inside the period ahead of
			// work that was never scheduled. The horizon is the sentinel: any floor at
			// or past it lands the dependent here too, and the block cascades.
			commitOf[in.init.Name] = horizon
			// bindingConstraint keeps whatever gate actually refused the release --
			// "wip-limit" or "pod-capacity" says why it could not get in, which is
			// more use than a generic "horizon" and needs no new enum value (FR-021).
			results[in.init.Name] = &si
			continue
		}

		for _, s := range placed {
			c := calendarFor(s.Pod)
			if c.tracks <= 0 || s.FinishWeek <= s.StartWeek {
				continue // unknown pods consume nothing; zero-length slices occupy nothing
			}
			if len(s.Phases) > 0 {
				continue // split slices reserved their weeks in splitPlace; booking
				// them again would double-count every phase lane
			}
			// The work end, distinct from the finish: a block-finish window
			// moves the completion without adding work, and a reduce-capacity
			// window stretches the span — both add waiting, not work. Count
			// working weeks (calendar weeks the rules leave at full tracks) to
			// find where the estimated work actually ends.
			workEnd, worked := s.StartWeek, 0
			for workEnd < s.FinishWeek && worked < int(s.RemainingWeeks) {
				if rules == nil || rules.reducedTracks(s.Pod, siteOf(teams, s.Pod), c.tracks, workEnd) > 0 {
					worked++
				}
				workEnd++
			}
			// Split slices (spec 007) occupy their PHASE lanes each week; flat
			// slices keep LanesUsed. workEnd is capped at RemainingWeeks of
			// consumption for flat slices; for split slices the phases already
			// encode exactly the consuming weeks (tax weeks included as ramp).
			weekLanes := func(w int) int {
				if len(s.Phases) == 0 {
					return s.LanesUsed
				}
				for _, ph := range s.Phases {
					if w >= ph.FromWeek && w < ph.ToWeek {
						return ph.Lanes
					}
				}
				return 0
			}
			sliceWorkEnd := workEnd
			if len(s.Phases) > 0 {
				sliceWorkEnd = s.FinishWeek // phases carry only consuming (and taxed ramp) weeks
			}
			for w := s.StartWeek; w < s.FinishWeek; w++ {
				if w >= sliceWorkEnd {
					continue // freeze/holiday waiting past the estimated work: not occupancy
				}
				// FR-018: a reduce-capacity week is not occupancy — a holiday
				// must not read as work in the heatmap or the flat rho.
				if rules != nil && rules.reducedTracks(s.Pod, siteOf(teams, s.Pod), c.tracks, w) <= 0 {
					continue
				}
				bumpInt(&c.busy, w, weekLanes(w))
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

// drumStagger is Decision 5's release stagger (spec 004 Story 5): refuse a
// release whose first week would push a drum pod this initiative works on to
// or above the target utilization. The occupancy read is what is already
// placed (the cal map mutates as initiatives are placed, so this sees exactly
// the load the earlier releases created). 0 or absent targetUtilization means
// no stagger — the inherited behaviour is unchanged.
func drumStagger(in *schedInput, sp SchedulingParams, cal map[string]*podCalendar, placed []WorkSlice) (bool, string) {
	if sp.TargetUtilization <= 0 || sp.TargetUtilization >= 1 || in.drumSet == nil {
		return false, ""
	}
	for _, s := range placed {
		if !in.drumSet[s.Pod] {
			continue
		}
		c := cal[s.Pod]
		if c == nil || c.tracks <= 0 {
			continue
		}
		// The bound is on OCCUPANCY, not start rate: this slice adds its
		// lanes in every week of its span, so refuse the release if any drum
		// week would then sit above the target. That makes the schedule's
		// actual drum load respect the target — a start-rate check cannot,
		// because slices from different releases overlap.
		// A target that admits less than one track would refuse every
		// placement forever, and the retry bound would then commit a slice that
		// violates it anyway — a silent violation is worse than no stagger, so
		// such a target is ignored for this pod rather than "enforced".
		if int(math.Ceil(sp.TargetUtilization*float64(c.tracks)-1e-9)) < 1 {
			continue
		}
		lanes := s.LanesUsed
		if lanes < 1 {
			lanes = 1
		}
		for w := s.StartWeek; w < s.FinishWeek; w++ {
			if float64(weekAt(c.busy, w)+lanes) > sp.TargetUtilization*float64(c.tracks)+1e-9 {
				return true, bindStagger
			}
		}
	}
	return false, ""
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
// splitPlace is spec 007's lane-splitting placement: the slice starts on the
// lanes free at its ready week (never zero), consumes effort at the lanes
// actually available each week, and grows as lanes free. The tax is charged
// as `tax` weeks of occupancy-with-no-consumption at the start of the run —
// the ramp a team pays to divide the work — so the arithmetic is:
// finish when Σ(lanes×weeks) over phases >= ceil(effort÷(1−loss)).
// Growth only, never preemption (Decision 1): weeks already claimed by this
// slice keep their lanes; the phase list is monotone non-decreasing.
func splitPlace(c *podCalendar, tracks, ready int, effort float64, loss float64, tax int, rules *calendarRules, pod, site string, _ bool) ([]LanePhase, string) {
	if c == nil || tracks <= 0 {
		return nil, "" // unknown-capacity pods cannot split; the caller falls back
	}
	total := effort / (1 - loss) // loss-stretched effort, exact this time
	phases := []LanePhase{}
	w := ready
	// The tax weeks run at the FIRST phase's lanes: the team is dividing the
	// work during them, so they occupy lanes without consuming effort.
		done := 0.0
	taxLeft := tax
	bound := ready + int(total) + tax + horizonBound // a pod cannot free lanes it never had
	for w <= bound {
		weekTracks := tracks
		if rules != nil {
			weekTracks = rules.reducedTracks(pod, site, tracks, w)
		}
		if weekTracks <= 0 {
			w++
			continue // non-working week: no consumption, no occupancy
		}
		free := weekTracks - weekAt(c.busy, w)
		if free <= 0 {
			// Busy week: CLOSE any open phase here — the gap is not ours to
			// claim, and leaving the phase open would swallow it (and later
			// double-book the pod when occupancy is booked).
			if len(phases) > 0 {
				phases[len(phases)-1].ToWeek = w
			}
			w++
			continue
		}
		prev := -1
		contiguous := false
		if n := len(phases); n > 0 {
			prev = phases[n-1].Lanes
			contiguous = phases[n-1].ToWeek == w
		}
		switch {
		case len(phases) == 0 || prev != free || !contiguous:
			phases = append(phases, LanePhase{FromWeek: w, ToWeek: w + 1, Lanes: free})
		default:
			phases[len(phases)-1].ToWeek = w + 1
		}
		if taxLeft > 0 {
			taxLeft--
			w++
			continue // occupying, ramping, not yet consuming
		}
		done += float64(free)
		w++
		if done >= total {
			phases[len(phases)-1].ToWeek = w
			return phases, ""
		}
	}
	// Could not finish inside the bound — signal the caller to fall back to
	// the all-or-nothing path rather than presenting a partial lie.
	return nil, "pod-capacity"
}

func planSlices(in *schedInput, release int, cal map[string]*podCalendar,
	teams map[string]Team, tracks map[string]int, perPodCap int, rules *calendarRules, sp SchedulingParams,
	capacityLoss float64) ([]WorkSlice, int, int) {

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
		site := siteOf(teams, pod)
		d := in.durations[pod]
		// Lanes (spec 006 Decision 2): computed before placement — the
		// capacity walk and the drum stagger both need to know how many
		// tracks this slice occupies, not just its duration.
		lanes := 1
		if sp.estimateModel() == EstimateEffort {
			effort := in.init.Work[pod].effortWeeks(in.init)
			need := int(math.Ceil(effort))
			// SplitMinWeeks caps the per-track load: 45 weeks with a 20-week
			// minimum chunks as 20+20+5 across 3 tracks, not 15×3.
			if sp.SplitMinWeeks > 0 {
				if effort < float64(sp.SplitMinWeeks) {
					need = 1 // below the threshold, work stays whole on one track
				} else {
					need = int(math.Ceil(effort / float64(sp.SplitMinWeeks)))
				}
			}
			if need > tracks[pod] && tracks[pod] > 0 {
				need = tracks[pod]
			}
			if need < 1 {
				need = 1
			}
			lanes = need
		}
		begin := ready
		sliceFinish := begin + d
		// Spec 007: when splitting is on (tax > 0, effort model), try the
		// growth placement first — start on the free lanes, absorb the rest
		// as they free, tax charged up front. Fall back to all-or-nothing
		// when the pod has no capacity or the walk could not finish.
		// Split only under contention: when the pod could satisfy all-or-nothing
		// at ready, the flat path is strictly better (no tax, no phase noise).
		// Splitting an unstarved slice buys nothing and costs the tax.
		needsSplit := false
		if sp.SplitTaxWeeks > 0 && d > 0 {
			effortForGate := in.init.Work[pod].effortWeeks(in.init)
			minOK := sp.SplitMinWeeks <= 0 || effortForGate >= float64(sp.SplitMinWeeks)
			if minOK {
				if c0 := cal[pod]; c0 != nil && tracks[pod] > 0 {
					freeAtReady := tracks[pod] - weekAt(c0.busy, begin)
					if lanes > freeAtReady {
						needsSplit = true
					}
				}
			}
		}
		if needsSplit {
			// The effort to spread is the progress-adjusted estimate in BOTH
			// models: under effort the sheet says total work; under wall-clock
			// the estimate is one lane's worth, and spreading it across free
			// lanes is exactly the split the planner asked for (225 single-lane
			// weeks onto a free track halves the time, plus tax).
			if effort := in.init.Work[pod].effortWeeks(in.init); effort > 0 {
				if phases, _ := splitPlace(cal[pod], tracks[pod], begin, effort, capacityLoss, sp.SplitTaxWeeks, rules, pod, site, in.init.InFlight); phases != nil {
					slices = append(slices, WorkSlice{
						Initiative: in.init.Name, Pod: pod,
						RemainingWeeks: float64(phases[len(phases)-1].ToWeek - phases[0].FromWeek),
						LanesUsed:      phases[0].Lanes, Phases: phases,
						StartWeek:      phases[0].FromWeek, FinishWeek: phases[len(phases)-1].ToWeek,
						WaitWeeks:      float64(phases[0].FromWeek - ready),
						BindingConstraint: reason, Estimated: in.init.Work[pod].Estimated && in.init.Work[pod].Weeks > 0,
						DependsOn:       append([]string(nil), in.deps[pod]...),
					})
					if phases[len(phases)-1].ToWeek > finish {
						finish = phases[len(phases)-1].ToWeek
					}
					if phases[0].FromWeek < start || i == 0 {
						start = phases[0].FromWeek
					}
					finishOf[pod] = phases[len(phases)-1].ToWeek
					continue
				}
			}
		}
		if rules != nil && d > 0 {
			// FR-018: no slice may begin in a frozen week — unless it is
			// carryover, which began before the period and cannot be
			// un-started (AC X.4). The freeze is the binding constraint only
			// when it moved the slice; otherwise whatever else delayed it
			// keeps the blame.
			if !in.init.InFlight {
				if s, moved := rules.firstStartFrom(pod, site, ready); moved {
					begin, reason = s, bindFreeze
				}
			}
			if d > 0 && tracks[pod] > 0 {
				// Returns both ends: a reduce-capacity window stretches the
				// span, so the finish is not simply begin + d any more. The
				// block-start rule is re-applied to every candidate inside —
				// capacity can otherwise push a start into a frozen week the
				// pre-check never saw.
				if s, f, why := firstFreeWeek(cal[pod], tracks[pod], begin, d, lanes, in.init.Name, perPodCap, rules, pod, site, in.init.InFlight); s > begin || f > sliceFinish {
					begin, sliceFinish = s, f
					if why != "" {
						reason = why
					}
				}
			}
			// FR-018: no completion inside a freeze. The finish moves to the
			// first legal week; the WORK does not grow — RemainingWeeks stays
			// the estimate, and the frozen weeks are waiting.
			if f, moved := rules.firstFinishFrom(pod, site, sliceFinish); moved {
				sliceFinish = f
				if reason == "" {
					reason = bindFreeze
				}
			}
		}
		w := in.init.Work[pod]
		slices = append(slices, WorkSlice{
			Initiative: in.init.Name, Pod: pod, RemainingWeeks: float64(d), LanesUsed: lanes,
			StartWeek: begin, FinishWeek: sliceFinish, WaitWeeks: float64(begin - ready),
			BindingConstraint: reason, Estimated: w.Estimated && w.Weeks > 0,
			DependsOn: append([]string(nil), in.deps[pod]...),
		})
		finishOf[pod] = sliceFinish
		if i == 0 || begin < start {
			start = begin
		}
		// sliceFinish, not begin+d: a reduce-capacity window stretches the
		// span beyond d working weeks, and the aggregate must follow the
		// stretch or the initiative reports a finish its own slice beats.
		if sliceFinish > finish {
			finish = sliceFinish
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
func firstFreeWeek(c *podCalendar, tracks, from, d, lanes int, initiative string, perPodCap int,
	rules *calendarRules, pod, site string, inFlight bool) (start int, finish int, reason string) {
	if lanes < 1 {
		lanes = 1
	}
	// The limit that refused the slice's own ready week is the one worth naming;
	// whatever refuses a later candidate week is a consequence of that first wait.
	refused := ""
	// The walk needs a bound: overlapping reduce-capacity windows are clipped to
	// the horizon at parse time, so any legal span ends within d + horizon weeks
	// of the earliest start. Without the bound, a fully-frozen pod would spin.
	ceiling := from + d + horizonBound
	for s := from; ; s++ {
		// The block-start rule applies to EVERY candidate, not just the ready
		// week: capacity can push a start into a frozen week the pre-check
		// never saw, and the schedule would then begin inside a freeze.
		// Carryover is exempt — it began before the period (AC X.4).
		if rules != nil && !inFlight && rules.startBlocked(pod, site, s) {
			continue
		}
		w, work := s, 0
		spanStart := -1 // the first week the slice actually works
		fits, why := true, ""
		for work < d && w <= ceiling {
			// The candidate check above guarded s, not w: a reduce-capacity
			// window can walk the span forward into a block-start week, and
			// that week must not become the recorded start. Only while no work
			// has begun — a freeze reached mid-span delays nothing that already
			// started; the rule forbids starts, not continuation.
			if spanStart < 0 && rules != nil && !inFlight && rules.startBlocked(pod, site, w) {
				fits, why = false, bindFreeze
				break
			}
			weekTracks := tracks
			if rules != nil {
				weekTracks = rules.reducedTracks(pod, site, tracks, w)
			}
			if weekTracks <= 0 {
				// A non-working week stretches the span but adds no work —
				// the team is on holiday, not re-scoping (FR-018).
				w++
				continue
			}
			// Multi-lane slices (spec 006) need `lanes` free tracks, not one:
			// an effort slice that occupies 3 lanes must find 3 free or wait.
			if c != nil && weekAt(c.busy, w)+lanes > weekTracks {
				fits, why = false, bindPodCapacity
				if weekTracks < tracks {
					why = bindFreeze
				}
				break
			}
			if perPodCap > 0 && c != nil && podWeekInitiatives(c, w, initiative) >= perPodCap {
				fits, why = false, bindPodWipLimit
				break
			}
			if spanStart < 0 {
				// The slice starts when its first working week starts, not
				// when its earliest holiday began: leading non-working weeks
				// are waiting, not work.
				spanStart = w
			}
			work++
			w++
		}
		if fits && work >= d {
			if spanStart < 0 {
				spanStart = s
			}
			return spanStart, w, refused
		}
		if fits && work < d {
			// The bound ran out before d working weeks existed: nothing fits,
			// and reporting a start would promise work the period cannot hold.
			return s, s + d, bindFreeze
		}
		if refused == "" {
			refused = why
		}
	}
}

// horizonBound is the walk ceiling above; generous rather than exact, because
// the exact bound (horizon + all frozen weeks) costs another pass to compute
// and the loop exits on the first fitting span long before this in practice.
const horizonBound = 208

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

// fitOf is Decision 28's arithmetic: what the plan asks of the period against what
// the period can absorb. Demand counts every initiative's in-path work, including
// the ones that did not fit -- they are the demand, and leaving them out would
// report a plan that fits.
func fitOf(inits []Initiative, sis []ScheduledInitiative, tracks map[string]int,
	params Params, horizon int) *ScheduleFit {
	fit := &ScheduleFit{}
	// Demand comes from the inputs, not the placed slices: an initiative held out
	// of the period has no slices, and counting only what was placed would report a
	// plan that fits. That is the whole question this field answers.
	for _, it := range inits {
		for _, w := range it.Work {
			if w.InPath {
				fit.PodWeeksDemanded += w.Weeks
			}
		}
	}
	for _, si := range sis {
		if si.Verdict == verdictBeyondHorizon {
			fit.BeyondHorizon++
			// Why it could not get in. At 6% capacity load a WIP limit, not the
			// pods, is what held it out, and reporting only demand-vs-capacity
			// would point the planner at the wrong lever.
			if si.BindingConstraint != "" {
				fit.HeldBy = appendCount(fit.HeldBy, si.BindingConstraint)
			}
		}
	}
	total := 0
	for _, tr := range tracks {
		total += tr
	}
	// The loss is why this is capacity rather than nameplate: a pod at three tracks
	// does not deliver three track-weeks a week (Params.CapacityLoss).
	fit.TrackWeeksAvailable = float64(total) * float64(horizon) * (1 - params.WithDefaults().CapacityLoss)
	return fit
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
	// FR-024 fever point, computed at the target week. No target or no buffer
	// means nothing to burn against, so the point stays at the origin.
	if si.BufferWeeks > 0 && in.targetWeek != nil {
		chain := finish - start
		if t := *in.targetWeek; chain > 0 {
			si.TargetProgress = math.Max(0, math.Min(1, float64(t-start)/float64(chain)))
			// The date consumes buffer whenever it lands before the buffered
			// commit — including before the chain even starts, where progress is
			// 0 and the miss is measured from the commit, not the finish.
			if t < si.CommitWeek {
				si.TargetBurn = float64(si.CommitWeek-t) / float64(si.BufferWeeks)
				if si.TargetProgress > 0 {
					si.BurnRatio = si.TargetBurn / si.TargetProgress
				} else {
					si.BurnRatio = si.TargetBurn // nothing started: the burn is the whole story
				}
			}
		}
	}

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

// annotateIdle fills the AC 4.2 comparison (spec 004 Story 4) on each pod: the
// mean weekly utilization the schedule produced, the flat rho the Network view
// reports, and the idle track-weeks bucketed by cause. The buckets are honest
// about what they can see: a calendar week is counted from the rules; an
// upstream week is one where the pod's next slice is waiting on a predecessor
// elsewhere; the remainder is everything the release gates hold. They are
// attributions, not counterfactuals.
func annotateIdle(sched *Schedule, rules *calendarRules, teams map[string]Team, loads []PodLoad, horizon int) {
	rho := map[string]float64{}
	for _, l := range loads {
		rho[l.Team] = l.Rho
	}
	// readyAt[pod] = the latest week by which the pod's own work becomes READY:
	// for every slice, the finish of the upstream slice it names (the
	// predecessor's completion), not the dependent slice's own finish. Waiting
	// attributed to a week the predecessor had already finished was never a
	// dependency wait.
	readyAt := map[string]int{}
	for _, ps := range sched.PodWeeks {
		for _, s := range ps.Slices {
			for _, up := range s.DependsOn {
				for _, ups := range sched.PodWeeks {
					if ups.Pod != up {
						continue
					}
					for _, us := range ups.Slices {
						if us.Initiative == s.Initiative && us.FinishWeek > readyAt[s.Pod] {
							readyAt[s.Pod] = us.FinishWeek
						}
					}
				}
			}
		}
	}
	for i := range sched.PodWeeks {
		ps := &sched.PodWeeks[i]
		if len(ps.Weeks) == 0 {
			continue
		}
		sum, n := 0.0, 0
		for _, pw := range ps.Weeks[:min(horizon, len(ps.Weeks))] {
			sum += pw.Utilization
			n++
			idle := float64(pw.Tracks - pw.Busy)
			if idle <= 0 {
				continue
			}
			w := pw.Week
			site := siteOf(teams, ps.Pod)
			if rules != nil && (rules.reducedTracks(ps.Pod, site, ps.Tracks, w) < ps.Tracks ||
				rules.startBlocked(ps.Pod, site, w) || rules.finishBlocked(ps.Pod, site, w)) {
				ps.Idle.Calendar += idle
				continue
			}
			// Upstream: the pod's work is not ready yet at w because a
			// predecessor slice has not finished.
			if w < readyAt[ps.Pod] {
				ps.Idle.Upstream += idle
				continue
			}
			ps.Idle.HeldForRelease += idle
		}
		if n > 0 {
			ps.MeanUtil = round1(sum/float64(n)*100) / 100
		}
		ps.FlatRho = round1(rho[ps.Pod]*100) / 100
	}
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
			ProposedRank: si.ProposedRank, Reason: reason, Locked: si.PriorityLocked})
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

// siteOf is the roster's site for a pod, lowercased for the calendar scope
// join — the same case-insensitivity every other pod-name match uses.
func siteOf(teams map[string]Team, pod string) string {
	return strings.ToLower(strings.TrimSpace(teams[pod].Site))
}
