package game

import "testing"

func fakeOrg() ([]PodInfo, map[string]PodStat, map[string]map[string]float64, []*Edge) {
	pods := []PodInfo{
		{"Alpha", "San Mateo", true, 3, 6},
		{"Beta", "Krakow", false, 4, 4},
		{"Cooperstown", "Remote", true, 3, 6},
	}
	stats := map[string]PodStat{
		"Alpha":       {Wip: 4, ThroughputWk: 4, Mu: 1.8, Sigma: 0.6, Rho0: 0.6, P50: 6, P85: 14, HygieneScore: 0.5},
		"Beta":        {Wip: 20, ThroughputWk: 3, Mu: 2.1, Sigma: 0.7, Rho0: 0.9, P50: 8, P85: 20, HygieneScore: 0.3},
		"Cooperstown": {Wip: 6, ThroughputWk: 5, Mu: 1.6, Sigma: 0.5, Rho0: 0.5, P50: 5, P85: 12, HygieneScore: 0.6},
	}
	overlap := map[string]map[string]float64{
		"Alpha":       {"Alpha": 8, "Beta": 0, "Cooperstown": 2},
		"Beta":        {"Alpha": 0, "Beta": 8, "Cooperstown": 2},
		"Cooperstown": {"Alpha": 2, "Beta": 2, "Cooperstown": 8},
	}
	edges := []*Edge{{From: "Beta", To: "Alpha", Count: 6}}
	return pods, stats, overlap, edges
}

func newFake(seed, rounds, ap int) *Game {
	p, s, o, e := fakeOrg()
	return NewGame(p, s, o, e, seed, rounds, ap, map[string]bool{"Cooperstown": true})
}

func TestNewGameSnapshot(t *testing.T) {
	g := newFake(99, 4, 3)
	if len(g.Pods) != 3 {
		t.Fatalf("want 3 pods, got %d", len(g.Pods))
	}
	if g.ApLeft != 3 || g.Round != 1 {
		t.Fatalf("bad init: ap=%d round=%d", g.ApLeft, g.Round)
	}
	// starting loads are normalized into [0.6, 1.5]; the coolest pod sits at 0.6
	for _, p := range g.Pods {
		if l := RhoOf(p); l < 0.59 || l > 1.51 {
			t.Fatalf("%s starting load %.2f out of band", p.Name, l)
		}
	}
}

func TestRhoUsesStreams(t *testing.T) {
	g := newFake(99, 4, 3)
	if RhoOf(g.Pods["Beta"]) <= 0.9 {
		t.Fatal("Beta should be hot")
	}
	if RhoOf(g.Pods["Alpha"]) >= 0.8 {
		t.Fatal("Alpha should be calm")
	}
}

func TestApBudget(t *testing.T) {
	g := newFake(99, 4, 3)
	if ok, _ := PlanMove(g, Move{Lever: "innovate", Pod: "Alpha", Flavor: "holistic"}); !ok {
		t.Fatal("innovate should apply")
	}
	if g.ApLeft != 1 {
		t.Fatalf("ap=%d, want 1", g.ApLeft)
	}
	if ok, _ := PlanMove(g, Move{Lever: "fullKitGate"}); ok {
		t.Fatal("fullKitGate(2) should exceed 1 AP left")
	}
}

func TestFreezeCools(t *testing.T) {
	g := newFake(99, 4, 3)
	w0 := g.Pods["Beta"].Wip
	b := RhoOf(g.Pods["Beta"])
	PlanMove(g, Move{Lever: "freeze", Pod: "Beta", N: 8})
	if g.Pods["Beta"].Wip != w0-8 {
		t.Fatalf("freeze should drop WIP by 8: %v -> %v", w0, g.Pods["Beta"].Wip)
	}
	if RhoOf(g.Pods["Beta"]) >= b {
		t.Fatal("freeze should cool the queue")
	}
}

func TestResolveDeterministic(t *testing.T) {
	a, c := newFake(7, 4, 3), newFake(7, 4, 3)
	ra, rc := ResolveRound(a), ResolveRound(c)
	if ra.ValueDelivered != rc.ValueDelivered || ra.Event != rc.Event {
		t.Fatal("same seed must replay identically")
	}
	if a.Round != 2 || a.ApLeft != 3 {
		t.Fatal("round/ap not advanced")
	}
}

func TestInterruptSuppressesDelivery(t *testing.T) {
	heavy, light := newFake(7, 4, 3), newFake(7, 4, 3)
	heavy.Pods["Alpha"].Interrupt = 12
	light.Pods["Alpha"].Interrupt = 0
	if ResolveRound(light).PerPod["Alpha"].Delivered <= ResolveRound(heavy).PerPod["Alpha"].Delivered {
		t.Fatal("interrupt burden should suppress delivery")
	}
}

func TestOpsDebtGrowsInterrupt(t *testing.T) {
	g := newFake(7, 4, 3)
	g.Pods["Alpha"].Readiness = 0.1
	g.Pods["Alpha"].Hygiene = 0.1
	before := g.Pods["Cooperstown"].Interrupt + g.Pods["Alpha"].Interrupt
	ResolveRound(g)
	ResolveRound(g)
	if g.Pods["Cooperstown"].Interrupt+g.Pods["Alpha"].Interrupt <= before {
		t.Fatal("ops debt should raise interrupt")
	}
}

func TestBurnoutErodesMorale(t *testing.T) {
	g := newFake(7, 4, 3)
	m0 := g.Pods["Beta"].Morale
	ResolveRound(g)
	ResolveRound(g)
	if g.Pods["Beta"].Morale >= m0 {
		t.Fatal("sustained overload should erode morale")
	}
}

func TestInnovationLongRun(t *testing.T) {
	quick, hol := newFake(7, 4, 3), newFake(7, 4, 3)
	PlanMove(quick, Move{Lever: "innovate", Pod: "Alpha", Flavor: "quickwin"})
	PlanMove(hol, Move{Lever: "innovate", Pod: "Alpha", Flavor: "holistic"})
	if ResolveRound(quick).ValueDelivered < ResolveRound(hol).ValueDelivered {
		t.Fatal("quick win should pay more now")
	}
	for i := 0; i < 3; i++ {
		ResolveRound(quick)
		ResolveRound(hol)
	}
	_, eq := FinalScore(quick)
	_, eh := FinalScore(hol)
	if eh.Total <= eq.Total {
		t.Fatalf("holistic should win long-run: %v !> %v", eh.Total, eq.Total)
	}
}

func TestScoreInRange(t *testing.T) {
	g := newFake(7, 4, 3)
	ResolveRound(g)
	s := scoreOf(g)
	for _, v := range []float64{s.ROI, s.Trust, s.Delight, s.TeamHealth, s.Innovation, s.Total} {
		if v < 0 || v > 100 {
			t.Fatalf("score component out of range: %v", v)
		}
	}
}

func TestEpiloguePunishesHollowOrg(t *testing.T) {
	healthy, hollow := newFake(7, 4, 3), newFake(7, 4, 3)
	for _, p := range hollow.Pods {
		p.Wip += 30
		p.Morale = 0.3
		p.Ktlo += 6
	}
	for i := 0; i < 4; i++ {
		ResolveRound(healthy)
		ResolveRound(hollow)
	}
	_, eh := FinalScore(healthy)
	_, eo := FinalScore(hollow)
	if eh.Total <= eo.Total {
		t.Fatal("hollow org should score worse in epilogue")
	}
}

func TestComboSynergy(t *testing.T) {
	g := newFake(7, 4, 12)
	PlanMove(g, Move{Lever: "freeze", Pod: "Beta", N: 8})
	PlanMove(g, Move{Lever: "wipCap", Pod: "Beta", CapX: 1})
	r := ResolveRound(g)
	good := false
	for _, c := range r.Combos {
		if c.Tone == "good" {
			good = true
		}
	}
	if !good {
		t.Fatal("freeze+cap should be a synergy")
	}
	if len(r.Story) < 60 {
		t.Fatal("story paragraph expected")
	}
}

func TestCurveBallCoverage(t *testing.T) {
	order := seededScenarioOrder(len(scenarios), 4, 20260613)
	seen := map[int]bool{}
	for _, i := range order {
		seen[i] = true
	}
	if len(order) != 4 || len(seen) != len(scenarios) {
		t.Fatalf("4-round game should cover all %d curve balls, got order %v", len(scenarios), order)
	}
}

func TestHygieneRevealsHiddenDeps(t *testing.T) {
	// across seeds, a hygiene sprint should sometimes uncover new inbound deps
	triggered := false
	for s := 1; s < 60 && !triggered; s++ {
		g := newFake(s, 4, 3)
		edges0 := len(g.Edges)
		PlanMove(g, Move{Lever: "hygieneSprint", Pod: "Alpha"})
		r := ResolveRound(g)
		if len(g.Edges) > edges0 {
			triggered = true
			found := false
			for _, sig := range r.Narrative {
				if contains(sig.Text, "uncovered") {
					found = true
				}
			}
			if !found {
				t.Fatal("a reveal should produce a narrative beat")
			}
			// the new edges should point INTO Alpha (inbound)
			inbound := 0
			for _, e := range g.Edges {
				if e.To == "Alpha" {
					inbound++
				}
			}
			if inbound < 1 {
				t.Fatal("revealed deps should be inbound to the sprinting pod")
			}
		}
	}
	if !triggered {
		t.Fatal("hygiene reveal never triggered across 60 seeds — probability too low?")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDifficultyPresets(t *testing.T) {
	p, st, o, e := fakeOrg()
	avgMorale := func(g *Game) float64 {
		s := 0.0
		for _, pod := range g.Pods {
			s += pod.Morale
		}
		return s / float64(len(g.Pods))
	}
	maxRho := func(g *Game) float64 {
		m := 0.0
		for _, pod := range g.Pods {
			if r := RhoOf(pod); r > m {
				m = r
			}
		}
		return m
	}
	bal := NewGameWith(p, st, o, e, 99, 4, 5, map[string]bool{"Cooperstown": true}, DifficultyFor("balanced"))
	cri := NewGameWith(p, st, o, e, 99, 4, 5, map[string]bool{"Cooperstown": true}, DifficultyFor("crisis"))
	if maxRho(cri) <= maxRho(bal) {
		t.Fatalf("crisis should start hotter: bal %.2f vs crisis %.2f", maxRho(bal), maxRho(cri))
	}
	if avgMorale(cri) >= avgMorale(bal) {
		t.Fatalf("crisis should start with lower morale: bal %.2f vs crisis %.2f", avgMorale(bal), avgMorale(cri))
	}
}
