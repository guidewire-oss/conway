package main

import (
	"sort"
	"testing"

	"conway/server/game"
)

// Headless balance diagnosis: run the real engine through a few strategies and
// print the WIP trend + final-score spread. Run with:
//
//	go test -run SimDiagnosis -v ./...
func TestSimDiagnosis(t *testing.T) {
	w := testWorld(t)
	const seed, rounds, ap = 20260613, 4, 5

	byRho := func(g *game.Game) []string {
		ns := append([]string{}, g.PodOrder...)
		sort.Slice(ns, func(i, j int) bool { return game.RhoOf(g.Pods[ns[i]]) > game.RhoOf(g.Pods[ns[j]]) })
		return ns
	}
	freezeN := func(g *game.Game, name string) float64 {
		p := g.Pods[name]
		c := p.WipCapX
		if c == 0 {
			c = 1
		}
		target := 0.7 * p.Streams * 2 * c
		if p.Wip > target {
			return p.Wip - target
		}
		return 0
	}

	strategies := map[string]func(g *game.Game) []game.Move{
		"do-nothing": func(g *game.Game) []game.Move { return nil },
		"naive-freeze-top1": func(g *game.Game) []game.Move {
			h := byRho(g)[0]
			return []game.Move{{Lever: "freeze", Pod: h, N: freezeN(g, h)}}
		},
		"cap-top5": func(g *game.Game) []game.Move {
			h := byRho(g)
			var m []game.Move
			for i := 0; i < 5 && i < len(h); i++ {
				m = append(m, game.Move{Lever: "wipCap", Pod: h[i], CapX: 0.8})
			}
			return m
		},
		"optimal-line": func(g *game.Game) []game.Move {
			// full mastery: cap hottest to healthy, hire the structural constraint,
			// compound with a holistic bet, raise readiness with hygiene, and commit
			// only pods already capped (so the dates actually hit).
			switch g.Round {
			case 1:
				return []game.Move{
					{Lever: "wipCap", Pod: "Danville", CapX: 0.8}, {Lever: "wipCap", Pod: "Kanata", CapX: 0.8},
					{Lever: "wipCap", Pod: "Bolinas", CapX: 0.8}, {Lever: "wipCap", Pod: "Okocim", CapX: 0.8},
					{Lever: "hire", Pod: "Kanata"},
				}
			case 2:
				return []game.Move{
					{Lever: "wipCap", Pod: "Avondale", CapX: 0.8}, {Lever: "wipCap", Pod: "Capitola", CapX: 0.8},
					{Lever: "wipCap", Pod: "Bandipur", CapX: 0.8},
					{Lever: "innovate", Pod: "Danville", Flavor: "holistic"}, // 2 AP
					{Lever: "commit", Pod: "Danville", DueRound: g.Round + 1},
				}
			case 3:
				return []game.Move{
					{Lever: "hygieneSprint", Pod: "Danville"}, {Lever: "hygieneSprint", Pod: "Kanata"},
					{Lever: "interfaceInvest", From: "Zakopane", To: "Danville"},
					{Lever: "interruptPolicy", Pod: "Krakow", Model: "followsun"},
					{Lever: "wipCap", Pod: byRho(g)[0], CapX: 0.8},
					{Lever: "commit", Pod: "Kanata", DueRound: g.Round + 1},
				}
			default:
				h := byRho(g)
				return []game.Move{
					{Lever: "wipCap", Pod: h[0], CapX: 0.8}, {Lever: "wipCap", Pod: h[1], CapX: 0.8},
					{Lever: "wipCap", Pod: h[2], CapX: 0.8}, {Lever: "wipCap", Pod: h[3], CapX: 0.8},
					{Lever: "wipCap", Pod: h[4], CapX: 0.8},
					{Lever: "commit", Pod: "Danville", DueRound: g.Round + 1}, {Lever: "commit", Pod: "Bolinas", DueRound: g.Round + 1},
				}
			}
		},
	}

	type fin struct {
		score game.Score
		ep    game.Epilogue
	}
	results := map[string]fin{}

	for name, plan := range strategies {
		g := game.NewGame(w.Pods, w.Stats, w.Overlap, w.freshEdges(), seed, rounds, ap, w.SrePods)
		t.Logf("\n========== STRATEGY: %s ==========", name)
		t.Logf(" round | totalWIP | avgRho | delivered | cost | score")
		for r := 1; r <= rounds; r++ {
			for _, m := range plan(g) {
				if ok, msg := game.PlanMove(g, m); !ok {
					t.Logf("   (move rejected: %s %s — %s)", m.Lever, m.Pod, msg)
				}
			}
			rep := game.ResolveRound(g)
			totWip, sumRho := 0, 0.0
			for _, pr := range rep.PerPod {
				totWip += pr.Wip
				sumRho += pr.Rho
			}
			n := float64(len(rep.PerPod))
			t.Logf("   %d   |  %5d   |  %4.2f  |  %7.1f  | %5.1f | %5.1f", rep.Round, totWip, sumRho/n, rep.ValueDelivered, rep.CostFn, rep.Score.Total)
		}
		sc, ep := game.FinalScore(g)
		results[name] = fin{sc, ep}
		t.Logf(" FINAL score=%.1f  (roi=%.0f trust=%.0f delight=%.0f team=%.0f innov=%.0f) | EPILOGUE=%.1f (morale=%.2f ktloShare=%.2f)",
			sc.Total, sc.ROI, sc.Trust, sc.Delight, sc.TeamHealth, sc.Innovation, ep.Total, ep.AvgMorale, ep.KtloShare)
	}

	t.Logf("\n========== SPREAD (final blended score) ==========")
	names := []string{"do-nothing", "naive-freeze-top1", "cap-top5", "optimal-line"}
	lo, hi := 1e9, -1e9
	for _, n := range names {
		s := results[n].score.Total
		if s < lo {
			lo = s
		}
		if s > hi {
			hi = s
		}
		t.Logf("  %-18s  %.1f", n, s)
	}
	t.Logf("  --> spread between best and worst: %.1f points", hi-lo)
}
