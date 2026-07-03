package game

import (
	"fmt"
	"math"
)

// ---- construction -------------------------------------------------------

// Difficulty tunes the starting world for a scenario: a wider/higher load band
// and lower starting morale make a game genuinely harder to solve.
type Difficulty struct {
	Name           string
	LoLoad, HiLoad float64 // starting load band (ρ)
	MoralePenalty  float64 // subtracted from starting morale
	Interrupt      float64 // base interrupt tax
	SreInterrupt   float64 // interrupt tax for SRE pods
	Ktlo           float64 // starting keep-the-lights-on burden
}

// Balanced is the default difficulty (winnable with good play).
func Balanced() Difficulty {
	return Difficulty{Name: "balanced", LoLoad: 0.6, HiLoad: 1.5, Interrupt: 2.0, SreInterrupt: 3.0, Ktlo: 1}
}

// DifficultyFor maps a scenario id to a preset (unknown / "default" / "jira" →
// Balanced over the mined world; "plan:*" is seeded elsewhere).
func DifficultyFor(scenario string) Difficulty {
	switch scenario {
	case "constrained":
		return Difficulty{Name: "constrained", LoLoad: 0.7, HiLoad: 1.7, MoralePenalty: 0.08, Interrupt: 2.5, SreInterrupt: 3.5, Ktlo: 1.5}
	case "crisis":
		return Difficulty{Name: "crisis", LoLoad: 0.85, HiLoad: 2.0, MoralePenalty: 0.18, Interrupt: 3.5, SreInterrupt: 4.5, Ktlo: 2.5}
	default:
		return Balanced()
	}
}

// NewGame builds a game at the default (Balanced) difficulty.
func NewGame(pods []PodInfo, stats map[string]PodStat, overlap map[string]map[string]float64,
	edges []*Edge, seed, totalRounds, apPerRound int, srePods map[string]bool) *Game {
	return NewGameWith(pods, stats, overlap, edges, seed, totalRounds, apPerRound, srePods, Balanced())
}

// NewGameWith builds a game with an explicit difficulty.
func NewGameWith(pods []PodInfo, stats map[string]PodStat, overlap map[string]map[string]float64,
	edges []*Edge, seed, totalRounds, apPerRound int, srePods map[string]bool, diff Difficulty) *Game {
	g := &Game{
		Seed: seed, Round: 1, TotalRounds: totalRounds,
		ApPerRound: apPerRound, ApLeft: apPerRound,
		Pods: map[string]*Pod{}, Overlap: overlap, Edges: edges,
	}
	// Normalize starting load into a realistic band. Mined WIP is so much larger
	// than a pod's healthy concurrency that a hard cap pins almost every pod at
	// the ceiling — round 1 becomes a wall of identical-overload pods no move can
	// touch. Instead, map raw loads onto [loLoad, hiLoad] preserving who's
	// relatively hotter, so a few pods start clearly hot and most are workable.
	loLoad, hiLoad := diff.LoLoad, diff.HiLoad
	streamsOf := func(pi PodInfo) float64 {
		if pi.Streams != 0 {
			return pi.Streams
		}
		if pi.Pairing {
			return pi.DevCount / 2
		}
		return pi.DevCount
	}
	minL, maxL := math.Inf(1), math.Inf(-1)
	for _, pi := range pods {
		s, ok := stats[pi.Name]
		if !ok {
			continue
		}
		l := s.Wip / math.Max(1, streamsOf(pi)*2)
		if l < minL {
			minL = l
		}
		if l > maxL {
			maxL = l
		}
	}
	span := maxL - minL

	for _, pi := range pods {
		s, ok := stats[pi.Name]
		if !ok {
			continue
		}
		streams := streamsOf(pi)
		interrupt := diff.Interrupt
		if srePods[pi.Name] {
			interrupt = diff.SreInterrupt
		}
		raw := s.Wip / math.Max(1, streams*2)
		targetLoad := 1.0
		if span > 0 {
			targetLoad = loLoad + (hiLoad-loLoad)*(raw-minL)/span
		}
		wip := targetLoad * streams * 2
		p := &Pod{
			Name: pi.Name, Location: pi.Location, Pairing: pi.Pairing,
			Streams: streams, DevCount: pi.DevCount, IsSre: srePods[pi.Name],
			Wip: wip, BaseThru: math.Max(0.5, s.ThroughputWk), Mu: s.Mu, Sigma: s.Sigma,
			Interrupt: interrupt, Ktlo: diff.Ktlo,
			// starting morale tracks the NORMALIZED load (not the huge mined rho0,
			// which would floor every pod at 0.4 and peg Team health at 0); the
			// scenario's penalty lowers it further for harder games.
			Morale:    clamp(0.85-0.4*math.Max(0, targetLoad-0.8)-diff.MoralePenalty, 0.3, 0.9),
			Hygiene:   clamp(s.HygieneScore, 0, 1),
			Readiness: clamp(0.35+0.5*s.HygieneScore, 0, 1),
			WipCapX:   0, ValuePerItem: 1, // 0 = uncapped; a WIP cap sets the ceiling multiple
		}
		g.Pods[pi.Name] = p
		g.PodOrder = append(g.PodOrder, pi.Name)
	}
	g.ScenarioOrder = seededScenarioOrder(len(scenarios), totalRounds, seed)
	return g
}

// ---- levers -------------------------------------------------------------

var leverAP = map[string]int{
	"freeze": 1, "wipCap": 1, "hygieneSprint": 1, "interfaceInvest": 1,
	"interruptPolicy": 1, "reassignScope": 1, "descopeMvp": 1, "fullKitGate": 2,
	"hire": 0, "innovate": 2, "commit": 0,
}

func PlanMove(g *Game, m Move) (bool, string) {
	ap, ok := leverAP[m.Lever]
	if !ok {
		return false, "unknown lever " + m.Lever
	}
	if ap > g.ApLeft {
		return false, fmt.Sprintf("not enough AP (need %d, have %d)", ap, g.ApLeft)
	}
	if err := applyLever(g, m); err != "" {
		return false, err
	}
	g.ApLeft -= ap
	m.Round = g.Round
	g.MoveLog = append(g.MoveLog, m)
	return true, ""
}

func applyLever(g *Game, m Move) string {
	switch m.Lever {
	case "freeze":
		p := g.Pods[m.Pod]
		if p == nil {
			return "unknown pod"
		}
		n := math.Min(m.N, p.Wip)
		p.Wip -= n
		p.FreezeChurn++
	case "wipCap":
		p := g.Pods[m.Pod]
		if p == nil {
			return "unknown pod"
		}
		p.WipCapX = clamp(orDefault(m.CapX, 1), 0.5, 3)
	case "hygieneSprint":
		p := g.Pods[m.Pod]
		if p == nil {
			return "unknown pod"
		}
		p.Hygiene = clamp(p.Hygiene+0.25, 0, 1)
		p.Wip = math.Max(0, p.Wip-math.Round(p.Wip*0.1))
		p.HygieneSprintRound = g.Round
		p.Readiness = deriveReadiness(p, g.FullKitGate)
	case "interfaceInvest":
		e := findEdge(g, m.From, m.To)
		if e == nil {
			return "unknown edge"
		}
		e.Interfaced = true
		e.Pending = 1
	case "interruptPolicy":
		p := g.Pods[m.Pod]
		if p == nil {
			return "unknown pod"
		}
		f := map[string]float64{"pool": 0.85, "office": 0.8, "followsun": 0.75, "dedicated": 1}[m.Model]
		if f == 0 {
			f = 1
		}
		p.Interrupt *= f
	case "reassignScope":
		from, to := g.Pods[m.From], g.Pods[m.To]
		if from == nil || to == nil {
			return "unknown pod"
		}
		frac := clamp(orDefault(m.Frac, 0.5), 0, 1)
		mw := math.Round(from.Wip * frac)
		mk := from.Ktlo * frac
		from.Wip -= mw
		from.Ktlo -= mk
		to.Wip += mw
		to.Ktlo += mk
		cross := from.Location != to.Location
		if cross {
			to.RampPenalty = 0.4
			to.RampRounds = 2
		} else {
			to.RampPenalty = 0.6
			to.RampRounds = 1
		}
	case "descopeMvp":
		p := g.Pods[m.Pod]
		if p == nil {
			return "unknown pod"
		}
		p.Mvp = true
		if m.CutOps {
			p.OpsCut = true
			p.Readiness = deriveReadiness(p, g.FullKitGate)
		}
	case "fullKitGate":
		g.FullKitGate = true
		for _, p := range g.Pods {
			p.Readiness = deriveReadiness(p, true)
		}
	case "hire":
		if g.HireUsed {
			return "backfill already placed this game"
		}
		p := g.Pods[m.Pod]
		if p == nil {
			return "unknown pod"
		}
		g.HireUsed = true
		p.PendingHire = 2
	case "innovate":
		p := g.Pods[m.Pod]
		if p == nil {
			return "unknown pod"
		}
		if m.Flavor == "quickwin" {
			p.ValuePerItem += 0.3
			p.Ktlo += 1.5
			p.QuickWin = true
			g.Tally.LocalDebt++
		} else {
			p.HolisticBet = true
			g.Tally.Holistic++
		}
	case "commit":
		g.Commitments = append(g.Commitments, &Commitment{Pod: m.Pod, DueRound: orDefaultInt(m.DueRound, g.Round+1), Value: 10})
		g.Tally.Committed++
	default:
		return "unknown lever"
	}
	return ""
}

// ---- resolution ---------------------------------------------------------

type evt struct {
	id string
	w  float64
}

var eventDeck = []evt{{"quiet", 3}, {"incidentWave", 2}, {"adoptionShock", 2}, {"demandSpike", 2}, {"execAsk", 1}}

func drawEvent(r func() float64) string {
	total := 0.0
	for _, e := range eventDeck {
		total += e.w
	}
	x := r() * total
	for _, e := range eventDeck {
		x -= e.w
		if x <= 0 {
			return e.id
		}
	}
	return "quiet"
}

func EventOf(seed, round int) string { return drawEvent(rng(hashSeed(seed, round))) }

func ResolveRound(g *Game) Report {
	r := rng(hashSeed(g.Seed, g.Round))
	event := drawEvent(r)
	perPod := map[string]PodResult{}
	valueDelivered, costFn := 0.0, 0.0

	type snap struct{ wip, morale, rho float64 }
	before := map[string]snap{}
	for _, p := range g.Pods {
		before[p.Name] = snap{p.Wip, p.Morale, RhoOf(p)}
	}
	frozen := map[string]float64{}
	var moves []Move
	for _, m := range g.MoveLog {
		if m.Round == g.Round {
			moves = append(moves, m)
			if m.Lever == "freeze" {
				frozen[m.Pod] += m.N
			}
		}
	}
	prevTotal := scoreOf(g).Total
	if len(g.History) > 0 {
		prevTotal = g.History[len(g.History)-1].Score.Total
	}
	var sig []Signal

	for _, name := range g.PodOrder {
		p := g.Pods[name]
		if p.Attrited && p.Streams <= 0 {
			perPod[name] = PodResult{}
			continue
		}
		if p.HolisticBet {
			p.InnovHolistic = math.Min(p.InnovHolistic+0.5, 3)
			p.Ktlo = math.Max(0.5, p.Ktlo-0.3)
		}

		devDaysWk := p.DevCount * 5
		burden := clamp((p.Interrupt+p.Ktlo)/devDaysWk, 0, 0.85)
		over := RhoOf(p)
		thrash := 1.0
		if over > 1 {
			thrash = clamp(1-0.12*(over-1), 0.45, 1)
		}
		ramp := 1.0
		if p.RampRounds > 0 {
			ramp = orOne(p.RampPenalty)
			p.RampRounds--
		}

		delivered := p.BaseThru * weeks * (1 - burden) * p.Morale * thrash * ramp
		if p.Mvp {
			delivered *= 1.3
		}
		if g.FullKitGate {
			delivered *= 0.9
		}
		hygieneSprint := p.HygieneSprintRound == g.Round
		if hygieneSprint {
			delivered *= 0.8
		}
		delivered = math.Max(0, delivered)

		innovBonus := 1 + 0.15*p.InnovHolistic
		valueDelivered += delivered * p.ValuePerItem * innovBonus

		demand := p.BaseThru * weeks * 0.72
		if event == "demandSpike" {
			demand *= 1.3
		}
		p.Wip = math.Max(0, p.Wip-delivered+demand)
		// a WIP cap is a real intake ceiling: WIP can't exceed capX x healthy
		// concurrency, so capping a pod hard-stops its inflation (TOC WIP limit).
		if p.WipCapX > 0 {
			if lim := p.WipCapX * p.Streams * 2; p.Wip > lim {
				p.Wip = lim
			}
		}

		g.Tally.ShipReadinessSum += p.Readiness * delivered
		g.Tally.ShipCount += delivered
		p.OpsDebt += delivered * (1 - p.Readiness) * 0.04
		adopt := p.OpsDebt * 0.5
		p.Interrupt += adopt * 0.6
		p.OpsDebt -= adopt * 0.6
		adoptSelf := adopt * 0.6
		for _, sre := range g.Pods {
			if sre.IsSre && sre != p {
				sre.Interrupt += adopt * 0.15
			}
		}

		rho := RhoOf(p)
		if rho > 0.9 {
			p.HiRhoStreak++
		} else {
			p.HiRhoStreak = 0
		}
		strain := afterHoursStrain(g, name)
		md := 0.0
		if p.HiRhoStreak >= 1 {
			md -= 0.04 * math.Min(float64(p.HiRhoStreak), 3)
		}
		if strain > 0 {
			md -= 0.02 * math.Min(strain, 6)
		}
		if p.FreezeChurn >= 2 {
			md -= 0.04
		}
		if rho < 0.85 && strain == 0 {
			md += 0.05
		}
		moraleDelta := clamp(p.Morale+md, 0.2, 0.95) - p.Morale
		p.Morale = clamp(p.Morale+md, 0.2, 0.95)
		p.FreezeChurn = 0

		attritedNow := false
		if p.Morale <= 0.42 && p.HiRhoStreak >= 2 && !p.Attrited {
			thresh := 0.5
			if p.Pairing {
				thresh = 0.25
			}
			if r() < thresh {
				p.Attrited = true
				p.Streams = math.Max(1, p.Streams-1)
				p.DevCount = math.Max(1, p.DevCount-pairDelta(p))
				p.Morale = clamp(p.Morale+0.1, 0.2, 0.95)
				g.Tally.Attritions++
				attritedNow = true
			}
		}

		// NEW CURVE BALL: a hygiene sprint can uncover latent inbound
		// dependencies — cleaning the house reveals coupling you weren't
		// tracking, putting the pod more squarely on the critical path.
		if hygieneSprint && r() < 0.45 {
			added := revealHiddenDeps(g, name, r)
			if added > 0 {
				sig = append(sig, Signal{"warn", fmt.Sprintf(
					"%s's hygiene sprint uncovered %d hidden inbound dependenc%s nobody was tracking — good to know, but it now sits more squarely on the critical path. Clean books reveal real coupling.",
					name, added, plural(added))})
			}
		}

		if p.QuickWin {
			p.Ktlo += 0.4
		}
		if p.PendingHire > 0 {
			p.PendingHire--
			if p.PendingHire == 0 {
				p.Streams++
				p.DevCount += pairDelta(p)
			}
		}

		// per-pod narrative
		bf := before[name]
		if attritedNow {
			sig = append(sig, Signal{"bad", name + " lost an engineer to burnout — morale had cratered under sustained overload. Capacity and knowledge took a hit."})
		} else if moraleDelta <= -0.08 {
			why := fmt.Sprintf("a hot queue (ρ %s)", dr(rho))
			if strain > 0 {
				why = "after-hours coordination across a zero-overlap seam"
			}
			sig = append(sig, Signal{"warn", fmt.Sprintf("%s morale fell to %s — %s. Left unchecked this trends toward attrition.", name, pctf(p.Morale), why)})
		} else if moraleDelta >= 0.05 {
			sig = append(sig, Signal{"good", fmt.Sprintf("%s morale recovered to %s as load eased.", name, pctf(p.Morale))})
		}
		if adoptSelf >= 0.8 {
			sig = append(sig, Signal{"warn", fmt.Sprintf("%s's interrupt load climbed to %.1f d/wk as earlier low-readiness work got adopted and started paging.", name, p.Interrupt)})
		}
		if over > 1.1 && delivered > 0 {
			sig = append(sig, Signal{"warn", fmt.Sprintf("%s is overloaded (ρ %s) — ~%s of throughput lost to context-switching. Freeze or cap its WIP.", name, dr(rho), pctf(1-thrash))})
		}
		if frozen[name] > 0 {
			sig = append(sig, Signal{"good", fmt.Sprintf("Freezing %s (×%.0f) kept its queue in hand (ρ %s) — flow over fullness.", name, frozen[name], dr(bf.rho))})
		}
		if p.HolisticBet && p.InnovHolistic >= 1 {
			sig = append(sig, Signal{"good", fmt.Sprintf("%s's holistic bet is compounding (+%s value, KTLO falling).", name, pctf(0.15*p.InnovHolistic))})
		}
		perPod[name] = PodResult{round2(delivered), round2(rho), round2(p.Morale), round1(p.Interrupt), int(math.Round(p.Wip))}
	}

	avgReadiness := 0.5
	if g.Tally.ShipCount > 0 {
		avgReadiness = g.Tally.ShipReadinessSum / g.Tally.ShipCount
	}
	switch event {
	case "incidentWave":
		hit := (1 - avgReadiness) * 8
		costFn += hit
		g.Tally.Incidents++
		sig = append(sig, Signal{"bad", fmt.Sprintf("A production incident wave hit (cost %.1f) — avg ship-readiness is only %d%%; observability and SRE-in-the-kit would have blunted it.", hit, int(avgReadiness*100))})
	case "adoptionShock":
		valueDelivered *= 1 + 0.15*avgReadiness
		paged := 0
		for _, p := range g.Pods {
			if p.Readiness < 0.5 {
				p.Interrupt += 1.5 * (1 - p.Readiness)
				paged++
			}
		}
		if avgReadiness > 0.6 {
			sig = append(sig, Signal{"good", fmt.Sprintf("A product caught on fast — high readiness (%d%%) turned adoption into value, not pages.", int(avgReadiness*100))})
		} else {
			sig = append(sig, Signal{"warn", fmt.Sprintf("A product caught on fast, but %d low-readiness pods started paging as usage climbed.", paged)})
		}
	case "execAsk":
		busy := busiest(g, 2)
		for _, p := range busy {
			p.Wip += 4
		}
		costFn += 1
		sig = append(sig, Signal{"warn", "An exec ask jumped the queue at " + joinNames(busy) + " — unplanned work the plan didn't have room for."})
	case "demandSpike":
		sig = append(sig, Signal{"warn", "Demand spiked — more work arrived than usual, pushing WIP up."})
	case "quiet":
		sig = append(sig, Signal{"info", "A quiet quarter — teams that built slack barely noticed."})
	}

	combos := detectCombos(g, moves)
	sig = append(sig, combos...)

	// whiplash
	disruptive := map[string]bool{"reassignScope": true, "interruptPolicy": true, "fullKitGate": true, "innovate": true}
	dc := 0
	for _, m := range moves {
		if disruptive[m.Lever] {
			dc++
		}
	}
	if dc >= 3 {
		for _, p := range g.Pods {
			p.Morale = clamp(p.Morale-0.06, 0.2, 0.95)
		}
		costFn += float64(dc) * 0.6
		sig = append(sig, Signal{"bad", fmt.Sprintf("Reorg whiplash: %d disruptive changes in one quarter. The org couldn't absorb them at once — morale dipped across pods. Change is itself a cost; sequence it.", dc)})
	}

	for _, e := range g.Edges {
		if e.Interfaced && e.Pending == 0 {
			continue
		}
		ov := g.Overlap[e.From][e.To]
		h := 1.5
		if ov >= 4 {
			h = 0.25
		} else if ov >= 2 {
			h = 0.5
		} else if ov > 0 {
			h = 1
		}
		costFn += float64(e.Count) * h * 0.4
		if e.Pending > 0 {
			e.Pending = 0
		}
	}
	for _, p := range g.Pods {
		rc := math.Min(RhoOf(p), rhoCap)
		if rc > 0.8 {
			costFn += rc / (1 - rc) * 0.8
		}
	}

	hit := 0
	for _, c := range g.Commitments {
		if c.DueRound == g.Round && !c.Resolved {
			c.Resolved = true
			p := g.Pods[c.Pod]
			if p != nil && RhoOf(p) < 0.9 && p.Morale > 0.4 {
				g.Tally.CommittedHit++
				hit++
			}
		}
	}
	if hit > 0 {
		sig = append(sig, Signal{"good", fmt.Sprintf("Hit %d committed date(s) — trust compounds.", hit)})
	}

	g.Tally.Value += valueDelivered
	g.Tally.CostFn += costFn
	score := scoreOf(g)
	delta := round1(score.Total - prevTotal)

	var watch []string
	for _, p := range busiest(g, 2) {
		if RhoOf(p) > 1.1 {
			watch = append(watch, fmt.Sprintf("%s stays overloaded (ρ %s) — freeze or cap WIP.", p.Name, dr(RhoOf(p))))
		}
	}

	headline := fmt.Sprintf("Roughly flat (%+.1f) — trade-offs cancelled out.", delta)
	if delta > 4 {
		headline = fmt.Sprintf("Strong quarter (+%.1f) — the system is flowing.", delta)
	} else if delta > 0 {
		headline = fmt.Sprintf("Net positive quarter (+%.1f).", delta)
	} else if delta <= -4 {
		headline = fmt.Sprintf("Tough quarter (%.1f) — something is dragging the system.", delta)
	}

	rep := Report{
		Round: g.Round, Event: event, ValueDelivered: round2(valueDelivered), CostFn: round2(costFn),
		CommitmentsHit: hit, Score: score, ScoreDelta: delta, Headline: headline,
		Story: buildStory(moves, sig, combos, delta), Combos: combos,
		Narrative: capSignals(sig, 9), Watch: capStrings(watch, 4), PerPod: perPod,
	}
	g.History = append(g.History, rep)
	g.Round++
	g.ApLeft = g.ApPerRound
	return rep
}

// revealHiddenDeps adds 1–2 inbound edges to `name` from other pods that don't
// already point at it — latent coupling surfaced by cleaning the books.
func revealHiddenDeps(g *Game, name string, r func() float64) int {
	existing := map[string]bool{}
	for _, e := range g.Edges {
		if e.To == name {
			existing[e.From] = true
		}
	}
	var cands []string
	for _, n := range g.PodOrder {
		if n != name && !existing[n] {
			cands = append(cands, n)
		}
	}
	if len(cands) == 0 {
		return 0
	}
	want := 1
	if r() < 0.5 && len(cands) > 1 {
		want = 2
	}
	added := 0
	for i := 0; i < want && len(cands) > 0; i++ {
		idx := int(r() * float64(len(cands)))
		if idx >= len(cands) {
			idx = len(cands) - 1
		}
		g.Edges = append(g.Edges, &Edge{From: cands[idx], To: name, Count: 2})
		cands = append(cands[:idx], cands[idx+1:]...)
		added++
	}
	return added
}
