package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

func soft(x, mid float64) float64 { return clamp(100*(x/(x+mid)), 0, 100) }

func scoreOf(g *Game) Score {
	t := g.Tally
	roi := soft(math.Max(0, t.Value-t.CostFn), 1500)
	// Trust: neutral 60 if you never commit. Committing is a positive-expected
	// skill lever — each kept date earns more than a missed one costs, so one
	// curve-ball-spoiled miss isn't fatal, but reliable promises clearly win.
	trust := 60.0
	if t.Committed > 0 {
		miss := t.Committed - t.CommittedHit
		trust = clamp(60+12*float64(t.CommittedHit)-8*float64(miss), 0, 100)
	}
	avgReadiness := 0.5
	if t.ShipCount > 0 {
		avgReadiness = t.ShipReadinessSum / t.ShipCount
	}
	delight := clamp(avgReadiness*100-float64(t.Incidents)*8, 0, 100)
	mt := 0.0
	for _, p := range g.Pods {
		mt += p.Morale
	}
	avgMorale := mt / math.Max(1, float64(len(g.Pods)))
	teamHealth := clamp(avgMorale*100-float64(t.Attritions)*12, 0, 100)
	innovation := clamp(50+12*float64(t.Holistic)-10*float64(t.LocalDebt), 0, 100)
	total := 0.25*roi + 0.15*trust + 0.10*delight + 0.15*teamHealth + 0.10*innovation + 0.25*50
	return Score{round1(roi), round1(trust), round1(delight), round1(teamHealth), round1(innovation), round1(total)}
}

func RunEpilogue(g *Game) Epilogue {
	value, cost := 0.0, 0.0
	type pc struct{ Pod }
	pods := make([]Pod, 0, len(g.Pods))
	for _, n := range g.PodOrder {
		pods = append(pods, *g.Pods[n])
	}
	r := rng(hashSeed(g.Seed, 999))
	for q := 0; q < 4; q++ {
		for i := range pods {
			p := &pods[i]
			if p.Attrited && p.Streams <= 0 {
				continue
			}
			devDaysWk := p.DevCount * 5
			burden := clamp((p.Interrupt+p.Ktlo)/devDaysWk, 0, 0.9)
			over := RhoOf(p)
			thrash := 1.0
			if over > 1 {
				thrash = clamp(1-0.12*(over-1), 0.4, 1)
			}
			innov := 1 + 0.15*p.InnovHolistic
			delivered := math.Max(0, p.BaseThru*weeks*(1-burden)*p.Morale*thrash)
			value += delivered * p.ValuePerItem * innov
			p.OpsDebt += delivered * (1 - p.Readiness) * 0.04
			p.Interrupt += p.OpsDebt * 0.3
			p.OpsDebt *= 0.7
			if r() < 0.15 {
				p.Ktlo += 1
			}
			rc := math.Min(RhoOf(p), rhoCap)
			if rc > 0.9 {
				p.Morale = clamp(p.Morale-0.05, 0.2, 0.95)
			} else {
				p.Morale = clamp(p.Morale+0.02, 0.2, 0.95)
			}
			if rc > 0.85 {
				cost += rc / (1 - rc) * 0.6
			}
		}
	}
	mt, kt := 0.0, 0.0
	for i := range pods {
		mt += pods[i].Morale
		kt += (pods[i].Interrupt + pods[i].Ktlo) / (pods[i].DevCount * 5)
	}
	n := math.Max(1, float64(len(pods)))
	avgMorale := mt / n
	ktloShare := kt / n
	flow := soft(math.Max(0, value-cost), 240)
	health := clamp(avgMorale*100, 0, 100)
	sustain := clamp(100*(1-ktloShare), 0, 100)
	total := round1(0.5*flow + 0.25*health + 0.25*sustain)
	return Epilogue{total, round1(value), round2(ktloShare), round2(avgMorale),
		epilogueNarrative(g, total, avgMorale, ktloShare)}
}

// epilogueNarrative is the "Dear player" letter: a plain-language read of how the
// org fares a year after the player stops, woven from their actual end-state and
// the choices tallied during play.
func epilogueNarrative(g *Game, total, avgMorale, ktloShare float64) string {
	t := g.Tally
	avgReadiness := 0.5
	if t.ShipCount > 0 {
		avgReadiness = t.ShipReadinessSum / t.ShipCount
	}
	s := []string{"Dear player, based on all the decisions you have made, here is how your organisation looks a year after you stepped away."}

	switch {
	case total >= 70:
		s = append(s, "The system you handed over kept flowing on its own — a year on, work still moves through it with less friction than the day you arrived, and it compounds the momentum you built rather than fighting it.")
	case total >= 50:
		s = append(s, "The organisation held its shape. It neither soared nor stalled; what you put in place was enough to keep it steady, if not yet self-improving.")
	case total >= 30:
		s = append(s, "The organisation is straining. The frictions you never resolved widened over the year, and the teams now spend more energy working around the system than through it.")
	default:
		s = append(s, "Left to run on its own, the organisation slowly unravelled. The cracks that were visible on your last day became the fault lines of the year that followed.")
	}

	switch {
	case avgMorale >= 0.7:
		s = append(s, "Your teams are still in good spirits — the sustainable pace you protected carried them through, and they have the slack to absorb whatever comes next.")
	case avgMorale >= 0.5:
		s = append(s, "Your teams are tired but intact, running closer to their limit than is comfortable.")
	default:
		s = append(s, "Your teams are running on fumes; the goodwill you spent has not come back, and every new ask now meets resistance.")
	}
	if t.Attritions > 0 {
		word := "teams"
		if t.Attritions == 1 {
			word = "team"
		}
		s = append(s, fmt.Sprintf("Along the way %d %s lost people to burnout, and the knowledge that walked out the door never fully returned.", t.Attritions, word))
	}

	switch {
	case ktloShare < 0.25:
		s = append(s, "Very little of each week is consumed keeping the lights on, so most of their capacity still goes to creating new value.")
	case ktloShare < 0.45:
		s = append(s, "A meaningful slice of every week now goes to keeping the lights on, quietly taxing everything else they try to do.")
	default:
		s = append(s, "Keep-the-lights-on work has crept up until it devours most of their week — there is little room left to build anything new.")
	}

	if t.Incidents > 0 || avgReadiness < 0.5 {
		s = append(s, "The unready work you shipped keeps paging the on-call, turning quiet weeks into firefights.")
	} else if avgReadiness >= 0.65 {
		s = append(s, "What you shipped was solid, so it largely runs itself — production rarely calls, and the teams stay on planned work.")
	}

	if t.Holistic > t.LocalDebt && t.Holistic > 0 {
		s = append(s, "The system-level bets you made are compounding — the automation and clean interfaces you invested in keep paying back long after you placed them.")
	} else if t.LocalDebt > 0 {
		s = append(s, "The quick wins you reached for hardened into debt the teams now route around — a tax on every change they attempt.")
	}

	if t.Committed > 0 {
		switch {
		case t.CommittedHit >= t.Committed:
			s = append(s, "Every date you committed to, you kept — and that reservoir of trust is the currency the org still spends to move fast.")
		case t.CommittedHit*2 >= t.Committed:
			s = append(s, "You kept some of the dates you promised and missed others; the org learned to half-believe your commitments.")
		default:
			s = append(s, "The promises you broke left a wariness that still colours every commitment made in your name.")
		}
	}

	switch {
	case total >= 70:
		s = append(s, "From here, the next leader inherits an organisation that wants to flow — their job is to keep clearing its path, not to rescue it.")
	case total >= 50:
		s = append(s, "From here it could tip either way: a little more attention to the constraint and the people and it improves; a little neglect and it slides.")
	case total >= 30:
		s = append(s, "From here it will take deliberate relief — less work in flight, fewer handoffs, room to breathe — before it can move forward again.")
	default:
		s = append(s, "From here, recovery is possible, but it starts with admitting how overloaded the system became and finding the courage to start less so it can finish more.")
	}
	return strings.Join(s, " ")
}

// FinalScore blends the played quarter with the epilogue (25%).
func FinalScore(g *Game) (Score, Epilogue) {
	s := scoreOf(g)
	e := RunEpilogue(g)
	played := 0.25*s.ROI + 0.15*s.Trust + 0.10*s.Delight + 0.15*s.TeamHealth + 0.10*s.Innovation
	s.Total = round1(played + 0.25*e.Total)
	return s, e
}

// ---- combos -------------------------------------------------------------

func detectCombos(g *Game, moves []Move) []Signal {
	by := map[string][]Move{}
	for _, m := range moves {
		by[m.Lever] = append(by[m.Lever], m)
	}
	var c []Signal
	for _, m := range by["freeze"] {
		for _, w := range by["wipCap"] {
			if w.Pod == m.Pod {
				if p := g.Pods[m.Pod]; p != nil {
					p.Morale = clamp(p.Morale+0.04, 0.2, 0.95)
				}
				c = append(c, Signal{"good", fmt.Sprintf("freezing %s and capping its WIP together didn't just clear the queue — it kept it clear, so the relief sticks", m.Pod)})
			}
		}
	}
	if g.FullKitGate && len(by["hygieneSprint"]) > 0 {
		for _, h := range by["hygieneSprint"] {
			if p := g.Pods[h.Pod]; p != nil {
				p.Readiness = clamp(p.Readiness+0.1, 0, 1)
			}
		}
		c = append(c, Signal{"good", "pairing the full-kit gate with a hygiene sprint compounded into genuine production-readiness, not just process"})
	}
	if len(by["interfaceInvest"]) > 0 && len(by["reassignScope"]) > 0 {
		for _, rm := range by["reassignScope"] {
			if to := g.Pods[rm.To]; to != nil && to.RampPenalty != 0 {
				to.RampPenalty = math.Min(1, to.RampPenalty+0.2)
			}
		}
		c = append(c, Signal{"good", "you built an interface before leaning on it — the seam you moved work across got cheaper, so the reassignment ramps faster"})
	}
	for _, m := range by["innovate"] {
		if m.Flavor != "quickwin" {
			for _, h := range by["hygieneSprint"] {
				if h.Pod == m.Pod {
					c = append(c, Signal{"good", fmt.Sprintf("betting on holistic automation in %s right after cleaning its house means the investment lands on solid ground", m.Pod)})
				}
			}
		}
	}
	for _, m := range by["freeze"] {
		for _, rm := range by["reassignScope"] {
			if rm.From == m.Pod || rm.To == m.Pod {
				if p := g.Pods[m.Pod]; p != nil {
					p.Morale = clamp(p.Morale-0.03, 0.2, 0.95)
				}
				c = append(c, Signal{"warn", fmt.Sprintf("freezing %s and reshuffling its work in the same breath mostly churned the same backlog — one decisive move beats two half-moves", m.Pod)})
			}
		}
	}
	for _, m := range by["interruptPolicy"] {
		p := g.Pods[m.Pod]
		if p != nil && (m.Model == "pool" || m.Model == "followsun" || m.Model == "office") && p.Readiness < 0.45 {
			p.OpsDebt += 1
			c = append(c, Signal{"warn", fmt.Sprintf("pooling %s's interrupts while it still ships at low readiness invites more novel pages, not fewer", m.Pod)})
		}
	}
	if len(by["hygieneSprint"]) >= 2 {
		c = append(c, Signal{"warn", "two hygiene sprints in one quarter spent a lot of delivery on cleanup at once — spreading them keeps flow steadier"})
	}
	return c
}

// ---- story --------------------------------------------------------------

func phrase(m Move) string {
	switch m.Lever {
	case "freeze":
		if m.N > 0 {
			return fmt.Sprintf("froze %s (×%.0f)", m.Pod, m.N)
		}
		return "froze " + m.Pod
	case "wipCap":
		return "capped WIP on " + m.Pod
	case "hygieneSprint":
		return "ran a hygiene sprint in " + m.Pod
	case "interfaceInvest":
		return fmt.Sprintf("invested in the %s→%s interface", m.From, m.To)
	case "interruptPolicy":
		return fmt.Sprintf("moved %s to a %s interrupt model", m.Pod, m.Model)
	case "reassignScope":
		return fmt.Sprintf("reassigned scope from %s to %s", m.From, m.To)
	case "descopeMvp":
		if m.CutOps {
			return "descoped " + m.Pod + " to an MVP (cutting ops)"
		}
		return "descoped " + m.Pod + " to an MVP"
	case "fullKitGate":
		return "turned on the org-wide full-kit gate"
	case "hire":
		return "placed a backfill in " + m.Pod
	case "innovate":
		if m.Flavor == "quickwin" {
			return "made a quick-win innovation bet in " + m.Pod
		}
		return "made a holistic innovation bet in " + m.Pod
	case "commit":
		return "committed a date for " + m.Pod
	}
	return m.Lever
}

func humanize(moves []Move) string {
	if len(moves) == 0 {
		return "held steady and made no structural changes"
	}
	ph := make([]string, len(moves))
	for i, m := range moves {
		ph[i] = phrase(m)
	}
	if len(ph) == 1 {
		return ph[0]
	}
	if len(ph) == 2 {
		return ph[0] + " and " + ph[1]
	}
	return strings.Join(ph[:len(ph)-1], ", ") + ", and " + ph[len(ph)-1]
}

func buildStory(moves []Move, sig, combos []Signal, delta float64) string {
	clean := func(t string) string { return strings.TrimRight(t, ".") }
	var goods, cautions []string
	for _, c := range combos {
		if c.Tone == "good" {
			goods = append(goods, clean(c.Text))
		} else {
			cautions = append(cautions, clean(c.Text))
		}
	}
	if len(goods) == 0 {
		for _, s := range sig {
			if s.Tone == "good" && len(goods) < 2 {
				goods = append(goods, clean(s.Text))
			}
		}
	}
	if len(cautions) == 0 {
		for _, s := range sig {
			if (s.Tone == "warn" || s.Tone == "bad") && len(cautions) < 2 {
				cautions = append(cautions, clean(s.Text))
			}
		}
	}
	if len(goods) > 2 {
		goods = goods[:2]
	}
	if len(cautions) > 2 {
		cautions = cautions[:2]
	}
	s := "This quarter you " + humanize(moves) + ". "
	if len(goods) > 0 {
		s += cap1(strings.Join(goods, "; and ")) + ". "
	}
	if len(cautions) > 0 {
		s += "The trade-off: " + strings.Join(cautions, "; ") + ". "
	}
	switch {
	case delta > 2:
		s += fmt.Sprintf("Net, the system moved with you (+%.1f) — keep pairing moves that reinforce each other.", delta)
	case delta >= -2:
		s += fmt.Sprintf("Net it roughly held (%+.1f) — find the one pairing that tips the balance rather than spreading effort thin.", delta)
	default:
		s += fmt.Sprintf("Net it slipped (%.1f) — read that as a signal, not a failure: ease the overloaded pods and let one change settle before stacking the next.", delta)
	}
	return s
}

// ---- scenarios (between-round curve balls) ------------------------------

type scenario struct {
	ID, Title, Text string
	apply           func(*Game)
}

var scenarios = []scenario{
	{"S1", "The whale feature", "A Tier-1 customer epic lands across your busiest pods, hard date next round.", func(g *Game) {
		busy := busiest(g, 4)
		for _, p := range busy {
			p.Wip += 8
		}
		if len(busy) > 0 {
			g.Commitments = append(g.Commitments, &Commitment{Pod: busy[0].Name, DueRound: g.Round + 1, Value: 20})
			g.Tally.Committed++
		}
	}},
	{"S2", "The 3 AM cascade", "A production incident chain hits — its cost scales with how much unready work you have shipped.", func(g *Game) {
		ar := 0.5
		if g.Tally.ShipCount > 0 {
			ar = g.Tally.ShipReadinessSum / g.Tally.ShipCount
		}
		hit := (1 - ar) * 10
		g.Tally.CostFn += hit
		g.Tally.Incidents++
		for _, p := range g.Pods {
			if p.IsSre {
				p.Interrupt += hit * 0.3
			}
		}
	}},
	{"S3", "Security mandate", "Compliance work lands on every pod simultaneously — non-negotiable.", func(g *Game) {
		for _, p := range g.Pods {
			p.Wip += 5
		}
	}},
	{"S6", "The optics demand", "Exec pressure for visible progress now.", func(g *Game) {
		for _, p := range g.Pods {
			p.Wip += 3
			p.Readiness = math.Max(0, p.Readiness-0.1)
		}
	}},
}

func seededScenarioOrder(n, count, seed int) []int {
	r := rng(uint32(seed) ^ 0x9e3779b9)
	var out []int
	for len(out) < count {
		a := make([]int, n)
		for i := range a {
			a[i] = i
		}
		for i := n - 1; i > 0; i-- {
			j := int(r() * float64(i+1))
			if j > i {
				j = i
			}
			a[i], a[j] = a[j], a[i]
		}
		out = append(out, a...)
	}
	return out[:count]
}

// ApplyScenario applies the curve ball for the interlude after the just-resolved
// round and returns its title/text (empty if none).
func ApplyScenario(g *Game) (string, string) {
	idx := g.Round - 2 // g.Round already advanced by ResolveRound
	if idx < 0 || idx >= len(g.ScenarioOrder) {
		return "", ""
	}
	sc := scenarios[g.ScenarioOrder[idx]]
	sc.apply(g)
	return sc.Title, sc.Text
}

// ---- small helpers ------------------------------------------------------

func findEdge(g *Game, from, to string) *Edge {
	for _, e := range g.Edges {
		if e.From == from && e.To == to {
			return e
		}
	}
	return nil
}
func busiest(g *Game, k int) []*Pod {
	var ps []*Pod
	for _, n := range g.PodOrder {
		if !g.Pods[n].Attrited {
			ps = append(ps, g.Pods[n])
		}
	}
	sort.Slice(ps, func(i, j int) bool { return RhoOf(ps[i]) > RhoOf(ps[j]) })
	if k > len(ps) {
		k = len(ps)
	}
	return ps[:k]
}
func joinNames(ps []*Pod) string {
	n := make([]string, len(ps))
	for i, p := range ps {
		n[i] = p.Name
	}
	return strings.Join(n, " and ")
}
func dr(x float64) string {
	if x > 3 {
		return "3+"
	}
	return fmt.Sprintf("%.1f", x)
}
func pctf(x float64) string { return fmt.Sprintf("%d%%", int(math.Round(x*100))) }
func cap1(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
func round1(x float64) float64 { return math.Round(x*10) / 10 }
func round2(x float64) float64 { return math.Round(x*100) / 100 }
func orDefault(x, d float64) float64 {
	if x == 0 {
		return d
	}
	return x
}
func orDefaultInt(x, d int) int {
	if x == 0 {
		return d
	}
	return x
}
func pairDelta(p *Pod) float64 {
	if p.Pairing {
		return 1
	}
	return 2
}
func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
func capSignals(s []Signal, n int) []Signal {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func capStrings(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
