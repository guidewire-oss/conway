// Package game is the AUTHORITATIVE Conway flow-game engine. It is the
// server-side port of the former client engine: the browser no longer holds
// the rules (scoring weights, hidden loops, synergies, thresholds), so they
// can't be read from source. The client sends moves and renders a sanitized
// view; all mechanics live here.
package game

import "math"

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

const weeks = 13.0
const rhoCap = 0.97

// deterministic PRNG (ported mulberry32) so a seed+round always plays the same
func rng(seed uint32) func() float64 {
	s := seed
	return func() float64 {
		s += 0x6D2B79F5
		t := s
		t = (t ^ (t >> 15)) * (t | 1)
		t ^= t + (t^(t>>7))*(t|61)
		return float64((t^(t>>14))&0xFFFFFFFF) / 4294967296.0
	}
}

func hashSeed(seed, round int) uint32 {
	// G115: truncation to 32 bits is the point — this is a hash mixer, and
	// wrap-around is how it spreads the seed. No arithmetic meaning is lost.
	return uint32((int64(seed) * 73856093) ^ (int64(round) * 19349663)) //nolint:gosec // G115: deliberate hash truncation
}

// ---- types --------------------------------------------------------------

type Pod struct {
	Name         string
	Location     string
	Pairing      bool
	Streams      float64
	DevCount     float64
	IsSre        bool
	Wip          float64
	BaseThru     float64 // throughput/week
	Mu, Sigma    float64
	Interrupt    float64 // dev-days/week
	Ktlo         float64
	Morale       float64
	Hygiene      float64
	Readiness    float64
	WipCapX      float64
	HiRhoStreak  int
	Attrited     bool
	ValuePerItem float64
	OpsDebt      float64
	OpsCut       bool
	// transient flags
	Mvp                bool
	HolisticBet        bool
	QuickWin           bool
	HygieneSprintRound int
	FreezeChurn        int
	RampPenalty        float64
	RampRounds         int
	InnovHolistic      float64
	PendingHire        int
}

type Edge struct {
	From, To   string
	Count      int
	Interfaced bool
	Pending    int
}

type Tally struct {
	Value, CostFn                      float64
	Committed, CommittedHit, Incidents int
	Holistic, LocalDebt, Attritions    int
	ShipReadinessSum, ShipCount        float64
}

type Commitment struct {
	Pod      string
	DueRound int
	Value    float64
	Resolved bool
}

type Game struct {
	Seed          int
	Round         int
	TotalRounds   int
	ApPerRound    int
	ApLeft        int
	Pods          map[string]*Pod
	PodOrder      []string
	Edges         []*Edge
	Overlap       map[string]map[string]float64
	FullKitGate   bool
	HireUsed      bool
	Tally         Tally
	Commitments   []*Commitment
	MoveLog       []Move
	Staged        []Move // moves planned for the open round, applied only on submit
	History       []Report
	ScenarioOrder []int
}

type Move struct {
	Round    int     `json:"round"`
	Lever    string  `json:"lever"`
	Pod      string  `json:"pod,omitempty"`
	From     string  `json:"from,omitempty"`
	To       string  `json:"to,omitempty"`
	N        float64 `json:"n,omitempty"`
	CapX     float64 `json:"capX,omitempty"`
	Frac     float64 `json:"frac,omitempty"`
	Model    string  `json:"model,omitempty"`
	Flavor   string  `json:"flavor,omitempty"`
	CutOps   bool    `json:"cutOps,omitempty"`
	DueRound int     `json:"dueRound,omitempty"`
}

type Signal struct {
	Tone string `json:"tone"`
	Text string `json:"text"`
}

type Score struct {
	ROI        float64 `json:"roi"`
	Trust      float64 `json:"trust"`
	Delight    float64 `json:"delight"`
	TeamHealth float64 `json:"teamHealth"`
	Innovation float64 `json:"innovation"`
	Total      float64 `json:"total"`
}

type Epilogue struct {
	Total        float64 `json:"total"`
	RunRateValue float64 `json:"runRateValue"`
	KtloShare    float64 `json:"ktloShare"`
	AvgMorale    float64 `json:"avgMorale"`
	Narrative    string  `json:"narrative"`
}

type Report struct {
	Round          int                  `json:"round"`
	Event          string               `json:"event"`
	ValueDelivered float64              `json:"valueDelivered"`
	CostFn         float64              `json:"costFn"`
	CommitmentsHit int                  `json:"commitmentsHit"`
	Score          Score                `json:"score"`
	ScoreDelta     float64              `json:"scoreDelta"`
	Headline       string               `json:"headline"`
	Story          string               `json:"story"`
	Narrative      []Signal             `json:"narrative"`
	Combos         []Signal             `json:"combos"`
	Watch          []string             `json:"watch"`
	PerPod         map[string]PodResult `json:"perPod"`
}

type PodResult struct {
	Delivered float64 `json:"delivered"`
	Rho       float64 `json:"rho"`
	Morale    float64 `json:"morale"`
	Interrupt float64 `json:"interrupt"`
	Wip       int     `json:"wip"`
}

// PodStat is the input shape from the mined org snapshot.
type PodStat struct {
	Wip, ThroughputWk, Mu, Sigma, Rho0, P50, P85 float64
	HygieneScore                                 float64
}
type PodInfo struct {
	Name, Location    string
	Pairing           bool
	Streams, DevCount float64
}

// ρ is honest load = WIP / healthy concurrency. A WIP cap no longer cosmetically
// lowers ρ; instead it bounds WIP intake (see ResolveRound), which lowers ρ for real.
func RhoOf(p *Pod) float64 { return p.Wip / math.Max(1, p.Streams*2) }
func orOne(x float64) float64 {
	if x == 0 {
		return 1
	}
	return x
}

func deriveReadiness(p *Pod, fullKit bool) float64 {
	r := 0.35 + 0.5*p.Hygiene
	if fullKit {
		r += 0.1
	}
	if p.OpsCut {
		r -= 0.3
	}
	return clamp(r, 0, 1)
}

func afterHoursStrain(g *Game, name string) float64 {
	strain := 0.0
	for _, e := range g.Edges {
		if e.Interfaced {
			continue
		}
		if e.From == name || e.To == name {
			other := e.To
			if e.From != name {
				other = e.From
			}
			if g.Overlap[name][other] <= 0 {
				strain += float64(e.Count)
			}
		}
	}
	return strain
}
