package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"conway/auth"
	"conway/game"
)

// testWorld builds a World from the fixture JSON under testdata/ — the same
// shapes buildWorld parses from a live snapshot's dynamic docs, but self-
// contained so tests don't need a Postgres connection.
func testWorld(t *testing.T) *World {
	t.Helper()
	w, err := buildWorld(func(name string, v any) error {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			return err
		}
		return json.Unmarshal(b, v)
	})
	if err != nil || len(w.Pods) == 0 {
		t.Fatalf("test world load: %v", err)
	}
	return w
}

func newTestServer(t *testing.T) *server {
	return &server{
		store:    auth.NewStore([]byte("x")),
		teams:    map[string]map[string]json.RawMessage{},
		games:    map[string]map[string]*game.Game{},
		sessions: map[string]*gameSession{defaultGameID: {Rounds: 4, Ap: 3, Open: true, OpenRound: 1}},
		world:    testWorld(t),
	}
}

// A nil s.world (the normal steady state with CONWAY_SEED_BASELINE=false and
// no snapshot literally named "baseline") must still let the default-game
// path fail with the correct, specific 503 — not be rejected earlier by a
// blanket check that would also (incorrectly) block a snap:/plan:-scenario
// game before it ever resolves its own world. See handleGameNew: the real
// "no world" check runs AFTER scenario resolution.
func TestGameNewNilWorldStillFailsCorrectly(t *testing.T) {
	s := newTestServer(t)
	s.world = nil
	c := auth.Claims{Sub: "t1", Roles: []string{"player"}}

	rec := httptest.NewRecorder()
	s.handleGameNew(rec, httptest.NewRequest("POST", "/api/game/new", nil), c)
	if rec.Code != 503 {
		t.Fatalf("expected 503 no-world for the default game, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no world loaded") {
		t.Fatalf("expected the specific 'no world loaded' error, got: %s", rec.Body.String())
	}
}

func TestGameEndpointsFlow(t *testing.T) {
	s := newTestServer(t)
	c := auth.Claims{Sub: "t1", Roles: []string{"player"}}

	rec := httptest.NewRecorder()
	s.handleGameNew(rec, httptest.NewRequest("POST", "/api/game/new", nil), c)
	var v gameView
	mustJSON(t, rec.Body.Bytes(), &v)
	if v.Round != 1 || v.ApLeft != 3 || len(v.Pods) == 0 {
		t.Fatalf("bad new view: %+v", v)
	}

	// stage a move — not applied yet, AP drops, it shows as staged
	stage := func(body string) (mv struct {
		Ok    bool
		Error string
		View  gameView
	}) {
		r := httptest.NewRecorder()
		s.handleStage(r, httptest.NewRequest("POST", "/api/game/stage", strings.NewReader(body)), c)
		mustJSON(t, r.Body.Bytes(), &mv)
		if strings.Contains(r.Body.String(), "opsDebt") || strings.Contains(r.Body.String(), "valuePerItem") {
			t.Fatal("client view leaked rule internals")
		}
		return
	}
	mv := stage(`{"lever":"freeze","pod":"` + v.Pods[0].Name + `","n":3}`)
	if !mv.Ok || mv.View.ApLeft != 2 || len(mv.View.MovesThisRound) != 1 {
		t.Fatalf("stage failed: %+v", mv)
	}

	// unstage it — AP restored, nothing applied
	rec = httptest.NewRecorder()
	s.handleUnstage(rec, httptest.NewRequest("POST", "/api/game/unstage", strings.NewReader(`{"index":0}`)), c)
	mustJSON(t, rec.Body.Bytes(), &mv)
	if mv.View.ApLeft != 3 || len(mv.View.MovesThisRound) != 0 {
		t.Fatalf("unstage failed: %+v", mv)
	}

	// re-stage and submit round 1
	stage(`{"lever":"wipCap","pod":"` + v.Pods[0].Name + `","capX":0.8}`)
	rec = httptest.NewRecorder()
	s.handleSubmit(rec, httptest.NewRequest("POST", "/api/game/submit", nil), c)
	var res struct {
		Report struct {
			Round           int
			Headline, Story string
		}
		ScenarioTitle string
		View          gameView
	}
	mustJSON(t, rec.Body.Bytes(), &res)
	if res.Report.Round != 1 || res.Report.Headline == "" {
		t.Fatalf("bad report: %+v", res.Report)
	}
	if res.View.Round != 2 {
		t.Fatalf("round should advance, got %d", res.View.Round)
	}
	if s.tmap(defaultGameID)["t1"] == nil {
		t.Fatal("submit should publish standings")
	}

	// can't submit round 2 until the facilitator opens it
	rec = httptest.NewRecorder()
	s.handleSubmit(rec, httptest.NewRequest("POST", "/api/game/submit", nil), c)
	if rec.Code != 409 {
		t.Fatalf("expected 409 before round 2 opens, got %d", rec.Code)
	}

	// facilitator opens each remaining round; team submits
	for round := 2; round <= 4; round++ {
		s.sess(defaultGameID).OpenRound = round
		rec = httptest.NewRecorder()
		s.handleSubmit(rec, httptest.NewRequest("POST", "/api/game/submit", nil), c)
	}
	rec = httptest.NewRecorder()
	s.handleGameGet(rec, httptest.NewRequest("GET", "/api/game", nil), c)
	mustJSON(t, rec.Body.Bytes(), &v)
	if !v.Over || v.Final == nil {
		t.Fatalf("game should end with a final score: %+v", v)
	}
}

func mustJSON(t *testing.T, b []byte, v any) {
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("json: %v\n%s", err, string(b))
	}
}

// A "tester" token (minted by a facilitator's 🧪 test button — see
// gamesadmin.go's "test" action) must play as freely as admin: it can start a
// game before the facilitator has opened round 1 for real teams, and can
// stage/submit moves even while the session is still closed to teams.
func TestTesterRoleBypassesRoundGate(t *testing.T) {
	s := newTestServer(t)
	gid := "game-under-trial"
	s.sessions[gid] = &gameSession{Rounds: 4, Ap: 3} // closed: Open=false, OpenRound=0
	c := auth.Claims{Sub: "__test__:facilitator1", Roles: []string{"tester"}, GameID: gid}

	rec := httptest.NewRecorder()
	s.handleGameNew(rec, httptest.NewRequest("POST", "/api/game/new", nil), c)
	if rec.Code != 200 {
		t.Fatalf("tester should start a game before round 1 opens, got %d: %s", rec.Code, rec.Body.String())
	}
	var v gameView
	mustJSON(t, rec.Body.Bytes(), &v)
	if v.Round != 1 {
		t.Fatalf("expected round 1, got %+v", v)
	}

	rec = httptest.NewRecorder()
	s.handleStage(rec, httptest.NewRequest("POST", "/api/game/stage", strings.NewReader(`{"lever":"freeze","pod":"`+v.Pods[0].Name+`","n":1}`)), c)
	var mv struct {
		Ok    bool
		Error string
	}
	mustJSON(t, rec.Body.Bytes(), &mv)
	if !mv.Ok {
		t.Fatalf("tester should be able to stage a move on a closed session, got error: %q", mv.Error)
	}

	// a plain player claim, same closed session, must still be blocked
	player := auth.Claims{Sub: "team-a", Roles: []string{"player"}, GameID: gid}
	rec = httptest.NewRecorder()
	s.handleGameNew(rec, httptest.NewRequest("POST", "/api/game/new", nil), player)
	if rec.Code != 409 {
		t.Fatalf("a plain player should still be blocked before round 1 opens, got %d", rec.Code)
	}
}

// A facilitator's 🧪 test session submits into the same tmap a real team would
// (so its own scoring works), but must never appear on the standings/
// leaderboard a real team's game is shown — it isn't a competitor.
func TestTesterSessionExcludedFromStandings(t *testing.T) {
	s := newTestServer(t)
	gid := "game-under-trial"
	s.sessions[gid] = &gameSession{Rounds: 4, Ap: 3}
	tester := auth.Claims{Sub: "__test__:facilitator1", Roles: []string{"tester"}, GameID: gid}
	realTeam := auth.Claims{Sub: "team-a", Roles: []string{"player"}, GameID: gid}

	rec := httptest.NewRecorder()
	s.handleGameNew(rec, httptest.NewRequest("POST", "/api/game/new", nil), tester)
	var v gameView
	mustJSON(t, rec.Body.Bytes(), &v)

	// open the round for real teams and have both submit round 1
	s.sess(gid).Open, s.sess(gid).OpenRound = true, 1
	rec = httptest.NewRecorder()
	s.handleGameNew(rec, httptest.NewRequest("POST", "/api/game/new", nil), realTeam)
	if rec.Code != 200 {
		t.Fatalf("real team should be able to start once round 1 is open, got %d: %s", rec.Code, rec.Body.String())
	}
	s.handleSubmit(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/game/submit", nil), tester)
	s.handleSubmit(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/game/submit", nil), realTeam)

	s.mu.Lock()
	st := s.standings(gid)
	s.mu.Unlock()

	if _, ok := st["team-a"]; !ok {
		t.Fatalf("real team should appear in standings: %+v", st)
	}
	if _, ok := st["__test__:facilitator1"]; ok {
		t.Fatalf("tester session leaked into standings/leaderboard: %+v", st)
	}
}

// round 1 on the REAL org should be a spread (a few hot pods, most workable),
// not a wall of pods all pinned at the same overload — the bug Anoop hit.
func TestStartingLoadIsSpreadNotClustered(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.handleGameNew(rec, httptest.NewRequest("POST", "/api/game/new", strings.NewReader(`{"rounds":4,"ap":3}`)), auth.Claims{Sub: "t1"})
	var v gameView
	mustJSON(t, rec.Body.Bytes(), &v)
	if len(v.Pods) < 10 {
		t.Skipf("only %d pods", len(v.Pods))
	}
	min, max := 99.0, 0.0
	atCeiling := 0
	for _, p := range v.Pods {
		if p.Rho < min {
			min = p.Rho
		}
		if p.Rho > max {
			max = p.Rho
		}
		if p.Rho >= 1.45 {
			atCeiling++
		}
	}
	if max-min < 0.5 {
		t.Fatalf("starting loads too clustered: min %.2f max %.2f", min, max)
	}
	if atCeiling > len(v.Pods)/3 {
		t.Fatalf("%d/%d pods pinned near ceiling — still a wall", atCeiling, len(v.Pods))
	}
}
