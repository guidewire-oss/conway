package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"conway/server/auth"
	"conway/server/game"
)

// --- sanitized client view: observable state only, never the rule internals
// (no opsDebt, valuePerItem, holisticBet, weights, thresholds, …) ---

type podView struct {
	Name      string  `json:"name"`
	Location  string  `json:"location"`
	Pairing   bool    `json:"pairing"`
	IsSre     bool    `json:"isSre"`
	Wip       int     `json:"wip"`
	Rho       float64 `json:"rho"`
	Morale    float64 `json:"morale"`
	Interrupt float64 `json:"interrupt"`
	Ktlo      float64 `json:"ktlo"`
	Readiness float64 `json:"readiness"`
	Hygiene   float64 `json:"hygiene"`
	Attrited  bool    `json:"attrited"`
}

type leverView struct {
	ID string `json:"id"`
	AP int    `json:"ap"`
}

type gameView struct {
	Round          int           `json:"round"`
	TotalRounds    int           `json:"totalRounds"`
	ApLeft         int           `json:"apLeft"`
	ApPerRound     int           `json:"apPerRound"`
	Over           bool          `json:"over"`
	Pods           []podView     `json:"pods"`
	Levers         []leverView   `json:"levers"`
	Edges          []edgeView    `json:"edges"`
	History        []game.Report `json:"history"`
	Score          game.Score    `json:"score"`
	Final          *finalView    `json:"final,omitempty"`
	MovesThisRound []game.Move   `json:"movesThisRound"`
}

type edgeView struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Count      int    `json:"count"`
	Interfaced bool   `json:"interfaced"`
}

type finalView struct {
	Score    game.Score    `json:"score"`
	Epilogue game.Epilogue `json:"epilogue"`
}

// AP costs are safe to expose (they're on the player briefing); the math isn't.
var clientLevers = []leverView{
	{"freeze", 1}, {"wipCap", 1}, {"hygieneSprint", 1}, {"interfaceInvest", 1},
	{"interruptPolicy", 1}, {"reassignScope", 1}, {"descopeMvp", 1}, {"fullKitGate", 2},
	{"hire", 0}, {"innovate", 2}, {"commit", 0},
}

var leverAPcost = func() map[string]int {
	m := map[string]int{}
	for _, l := range clientLevers {
		m[l.ID] = l.AP
	}
	return m
}()

func stagedAP(g *game.Game) int {
	ap := 0
	for _, m := range g.Staged {
		ap += leverAPcost[m.Lever]
	}
	return ap
}

func viewOf(g *game.Game) gameView {
	v := gameView{
		Round: g.Round, TotalRounds: g.TotalRounds, ApLeft: g.ApPerRound - stagedAP(g), ApPerRound: g.ApPerRound,
		Over: g.Round > g.TotalRounds, Levers: clientLevers, History: g.History,
	}
	for _, n := range g.PodOrder {
		p := g.Pods[n]
		v.Pods = append(v.Pods, podView{
			Name: p.Name, Location: p.Location, Pairing: p.Pairing, IsSre: p.IsSre,
			Wip: int(p.Wip + 0.5), Rho: round2(game.RhoOf(p)), Morale: round2(p.Morale),
			Interrupt: round1(p.Interrupt), Ktlo: round1(p.Ktlo), Readiness: round2(p.Readiness),
			Hygiene: round2(p.Hygiene), Attrited: p.Attrited,
		})
	}
	for _, e := range g.Edges {
		v.Edges = append(v.Edges, edgeView{e.From, e.To, e.Count, e.Interfaced})
	}
	v.MovesThisRound = g.Staged
	if v.Over {
		sc, ep := game.FinalScore(g)
		v.Score = sc
		v.Final = &finalView{Score: sc, Epilogue: ep}
	} else {
		v.Score = scoreView(g)
	}
	return v
}

func round1(x float64) float64 { return float64(int(x*10+0.5)) / 10 }
func round2(x float64) float64 { return float64(int(x*100+0.5)) / 100 }

// scoreView returns the in-progress snapshot (FinalScore is only at game end)
func scoreView(g *game.Game) game.Score {
	if len(g.History) > 0 {
		return g.History[len(g.History)-1].Score
	}
	return game.Score{}
}

// --- handlers ---

func (s *server) handleGameNew(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	// no early "s.world == nil" bailout here: a game may resolve its own world
	// from a specific snapshot/plan scenario below (snap:/plan:) without ever
	// touching s.world (the default-game/difficulty-preset world) — the real
	// check is after that resolution, at "if world == nil" below.
	gid := gameID(c)
	s.mu.Lock()
	sess := *s.sess(gid)
	exists := s.gmap(gid)[c.Sub] != nil
	s.mu.Unlock()
	// teams may only begin once the facilitator has opened round 1, and may NOT
	// restart once they have a game. admin/tester can always (re)start to test.
	if !c.CanTest() {
		if !sess.Open || sess.OpenRound < 1 {
			http.Error(w, "the facilitator has not opened round 1 yet", 409)
			return
		}
		if exists {
			http.Error(w, "you can't restart your game", 409)
			return
		}
	}
	// seed the starting world: a difficulty preset over the mined world, or a
	// world built from a saved plan (scenario "plan:<id>").
	world := s.world
	diff := game.Balanced()
	if gid != defaultGameID && s.db != nil {
		if gr, _ := s.db.GetGame(gid); gr != nil {
			switch {
			case strings.HasPrefix(gr.Scenario, "plan:"):
				if pw, err := s.planWorld(strings.TrimPrefix(gr.Scenario, "plan:")); err == nil && len(pw.Pods) > 0 {
					world = pw
				}
			case strings.HasPrefix(gr.Scenario, "snap:"):
				if sw, err := s.worldFromSnapshot(strings.TrimPrefix(gr.Scenario, "snap:")); err == nil && len(sw.Pods) > 0 {
					world = sw
				}
			default:
				diff = game.DifficultyFor(gr.Scenario)
			}
		}
	}
	if world == nil {
		http.Error(w, "no world loaded", 503)
		return
	}
	seed := 20260613
	g := game.NewGameWith(world.Pods, world.Stats, world.Overlap, world.freshEdges(),
		seed, sess.Rounds, sess.Ap, world.SrePods, diff)
	s.mu.Lock()
	s.gmap(gid)[c.Sub] = g
	v := viewOf(g)
	s.saveState()
	s.mu.Unlock()
	writeJSON(w, v)
}

func (s *server) handleGameGet(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	gid := gameID(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.gmap(gid)[c.Sub]
	if g == nil {
		writeJSON(w, map[string]any{"game": nil})
		return
	}
	writeJSON(w, viewOf(g))
}

// handleGameConfig returns the session (rounds/open/round/timer/deadline) for the
// caller's game — the per-game equivalent of /api/config for joined players.
func (s *server) handleGameConfig(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	gid := gameID(c)
	s.mu.Lock()
	sess := *s.sess(gid)
	s.mu.Unlock()
	writeJSON(w, map[string]any{"gameId": gid, "gameOpen": sess.Open, "rounds": sess.Rounds,
		"ap": sess.Ap, "openRound": sess.OpenRound, "timerSecs": sess.TimerSecs, "deadline": sess.Deadline})
}

// admin-only: read any team's sanitized game view in the default (legacy) game.
func (s *server) handleAdminGame(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	team := r.URL.Query().Get("team")
	if r.Method == http.MethodDelete {
		s.mu.Lock()
		delete(s.gmap(defaultGameID), team)
		delete(s.tmap(defaultGameID), team)
		s.saveState()
		s.mu.Unlock()
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.gmap(defaultGameID)[team]
	if g == nil {
		writeJSON(w, map[string]any{"game": nil})
		return
	}
	writeJSON(w, viewOf(g))
}

// canPlay reports whether the team's game is on the open round. Admin/tester
// bypasses the round gate (to test). Caller holds s.mu.
func (s *server) canPlay(gid string, g *game.Game, c auth.Claims) (bool, string) {
	sess := s.sess(gid)
	if g == nil {
		return false, "no game"
	}
	if c.CanTest() {
		return true, ""
	}
	if !sess.Open {
		return false, "the game is closed"
	}
	if sess.OpenRound < 1 {
		return false, "no round is open yet"
	}
	if g.Round != sess.OpenRound {
		return false, "round not open (already submitted, or waiting for the facilitator)"
	}
	return true, ""
}

func (s *server) handleStage(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	var m game.Move
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "bad move", 400)
		return
	}
	gid := gameID(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.gmap(gid)[c.Sub]
	if ok, msg := s.canPlay(gid, g, c); !ok {
		writeJSON(w, map[string]any{"ok": false, "error": msg, "view": viewOf(g)})
		return
	}
	cost, known := leverAPcost[m.Lever]
	if !known {
		writeJSON(w, map[string]any{"ok": false, "error": "unknown lever", "view": viewOf(g)})
		return
	}
	if stagedAP(g)+cost > g.ApPerRound {
		writeJSON(w, map[string]any{"ok": false, "error": "not enough Activity Points", "view": viewOf(g)})
		return
	}
	m.Round = g.Round
	g.Staged = append(g.Staged, m)
	s.saveState()
	writeJSON(w, map[string]any{"ok": true, "view": viewOf(g)})
}

func (s *server) handleUnstage(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	var body struct{ Index int }
	json.NewDecoder(r.Body).Decode(&body)
	gid := gameID(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.gmap(gid)[c.Sub]
	if ok, msg := s.canPlay(gid, g, c); !ok {
		writeJSON(w, map[string]any{"ok": false, "error": msg, "view": viewOf(g)})
		return
	}
	if body.Index >= 0 && body.Index < len(g.Staged) {
		g.Staged = append(g.Staged[:body.Index], g.Staged[body.Index+1:]...)
	}
	s.saveState()
	writeJSON(w, map[string]any{"ok": true, "view": viewOf(g)})
}

func (s *server) handleSubmit(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	gid := gameID(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.gmap(gid)[c.Sub]
	if ok, msg := s.canPlay(gid, g, c); !ok {
		http.Error(w, msg, 409)
		return
	}
	if g.Round > g.TotalRounds {
		writeJSON(w, map[string]any{"view": viewOf(g)})
		return
	}
	rep, scTitle, scText := s.submitRound(gid, g, c.Sub)
	s.saveState()
	writeJSON(w, map[string]any{
		"report": rep, "scenarioTitle": scTitle, "scenarioText": scText, "view": viewOf(g),
	})
}

// submitRound applies staged moves, resolves, and publishes standings to the
// game's board. Caller must hold s.mu.
func (s *server) submitRound(gid string, g *game.Game, team string) (game.Report, string, string) {
	for _, m := range g.Staged {
		game.PlanMove(g, m)
	}
	g.Staged = nil
	rep := game.ResolveRound(g)
	scTitle, scText := game.ApplyScenario(g)
	st, _ := json.Marshal(map[string]any{"round": g.Round, "score": rep.Score})
	s.tmap(gid)[team] = st
	return rep, scTitle, scText
}
