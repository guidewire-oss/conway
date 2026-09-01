package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
)

// --- per-game team roster (option A: facilitator pre-adds teams, each with a code) ---

func (s *server) addGameTeam(w http.ResponseWriter, r *http.Request, gid string) {
	var b struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&b)
	name := strings.TrimSpace(b.Name)
	if name == "" {
		http.Error(w, "enter a team name", 400)
		return
	}
	code := ""
	for i := 0; i < 10; i++ { // unique across team codes AND game codes
		cand := newJoinCode()
		t, _ := s.db.GameTeamByCode(cand)
		g, _ := s.db.GetGameByCode(cand)
		if t == nil && g == nil {
			code = cand
			break
		}
	}
	if code == "" {
		http.Error(w, "could not allocate a code", 500)
		return
	}
	if err := s.db.AddGameTeam(db.GameTeam{GameID: gid, Name: name, Code: code, CreatedAt: time.Now().Unix()}); err != nil {
		http.Error(w, "that team name is already in this game's roster", 409)
		return
	}
	writeJSON(w, map[string]any{"name": name, "code": code})
}

func (s *server) listGameTeams(w http.ResponseWriter, gid string) {
	roster, err := s.db.ListGameTeams(gid)
	if err != nil {
		s.logger().Error("request failed", "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	type row struct {
		Name   string `json:"name"`
		Code   string `json:"code"`
		Joined bool   `json:"joined"`
		Round  int    `json:"round"`
	}
	s.mu.Lock()
	gm := s.gmap(gid)
	out := make([]row, 0, len(roster))
	for _, t := range roster {
		rr := row{Name: t.Name, Code: t.Code}
		if g := gm[t.Name]; g != nil {
			rr.Joined, rr.Round = true, g.Round
		}
		out = append(out, rr)
	}
	s.mu.Unlock()
	writeJSON(w, out)
}

func (s *server) delGameTeam(w http.ResponseWriter, gid, name string) {
	if err := s.db.DeleteGameTeam(gid, name); err != nil {
		s.logger().Error("request failed", "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	s.mu.Lock()
	delete(s.gmap(gid), name)
	delete(s.tmap(gid), name)
	s.saveState()
	s.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

// handleGames: facilitator lists their games (admins: all) or creates one.
func (s *server) handleGames(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "multi-game requires the database", 503)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.ListGames(c.Sub, c.Has("admin"))
		if err != nil {
			s.logger().Error("request failed", "path", r.URL.Path, "err", err)
			http.Error(w, err.Error(), 500)
			return
		}
		// overlay the live run state (the registry's open/round columns are vestigial)
		s.mu.Lock()
		for i := range rows {
			if sess := s.sessions[rows[i].ID]; sess != nil {
				rows[i].Open, rows[i].OpenRound, rows[i].Deadline = sess.Open, sess.OpenRound, sess.Deadline
			}
		}
		s.mu.Unlock()
		writeJSON(w, rows)
	case http.MethodPost:
		var b struct {
			Name        string `json:"name"`
			Rounds      int    `json:"rounds"`
			Ap          int    `json:"ap"`
			TimerSecs   int    `json:"timerSecs"`
			ExpiryHours int    `json:"expiryHours"`
			Scenario    string `json:"scenario"` // seed (default for now; presets/jira/plan seed next)
		}
		json.NewDecoder(r.Body).Decode(&b)
		rounds, ap, timer := 4, 5, 300
		if b.Rounds > 0 {
			rounds = clampInt(b.Rounds, 1, 8)
		}
		if b.Ap > 0 {
			ap = clampInt(b.Ap, 2, 6)
		}
		if b.TimerSecs > 0 {
			timer = clampInt(b.TimerSecs, 30, 3600)
		}
		name := strings.TrimSpace(b.Name)
		if name == "" {
			http.Error(w, "enter a game name", 400)
			return
		}
		if taken, _ := s.db.GameNameTaken(c.Sub, name, ""); taken {
			http.Error(w, "you already have a game with that name", 409)
			return
		}
		now := time.Now().Unix()
		exp := int64(0)
		if b.ExpiryHours > 0 {
			exp = now + int64(b.ExpiryHours)*3600
		}
		code := ""
		for i := 0; i < 8; i++ { // retry on the rare collision
			cand := newJoinCode()
			if g, _ := s.db.GetGameByCode(cand); g == nil {
				code = cand
				break
			}
		}
		if code == "" {
			http.Error(w, "could not allocate a join code", 500)
			return
		}
		scenario := strings.TrimSpace(b.Scenario)
		if scenario == "" {
			scenario = "default"
		}
		g := db.GameRow{ID: newID(), Owner: c.Sub, Name: name, JoinCode: code,
			Rounds: rounds, Ap: ap, TimerSecs: timer, Scenario: scenario, CreatedAt: now, ExpiresAt: exp}
		if err := s.db.CreateGame(g); err != nil {
			s.logger().Error("request failed", "path", r.URL.Path, "err", err)
			http.Error(w, err.Error(), 500)
			return
		}
		s.mu.Lock()
		s.sessions[g.ID] = &gameSession{Rounds: rounds, Ap: ap, TimerSecs: timer}
		s.saveState()
		s.mu.Unlock()
		writeJSON(w, map[string]any{"id": g.ID, "joinCode": g.JoinCode, "name": g.Name})
	default:
		methodNotAllowed(w, r)
	}
}

// handleGameItem: GET / DELETE a single game (owner or admin).
func (s *server) handleGameItem(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "multi-game requires the database", 503)
		return
	}
	id, sub, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/games/"), "/")
	action, teamSeg, _ := strings.Cut(sub, "/")
	teamName, _ := url.PathUnescape(teamSeg)
	if id == "" {
		http.Error(w, "no game id", 400)
		return
	}
	g, err := s.db.GetGame(id)
	if err != nil {
		s.logger().Error("request failed", "path", r.URL.Path, "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	if g == nil {
		http.Error(w, "game not found", 404)
		return
	}
	if g.Owner != c.Sub && !c.Has("admin") {
		http.Error(w, "forbidden", 403)
		return
	}
	post, get := r.Method == http.MethodPost, r.Method == http.MethodGet
	switch {
	case action == "" && get:
		s.mu.Lock()
		sess := *s.sess(id)
		s.mu.Unlock()
		writeJSON(w, map[string]any{"game": g, "session": sess})
	case action == "" && r.Method == http.MethodPatch:
		s.editGame(w, r, g)
	case action == "" && r.Method == http.MethodDelete:
		s.db.DeleteGame(id)
		s.mu.Lock()
		delete(s.games, id)
		delete(s.teams, id)
		delete(s.sessions, id)
		s.saveState()
		s.mu.Unlock()
		writeJSON(w, map[string]any{"ok": true})
	case action == "teams" && get && teamName == "":
		s.listGameTeams(w, id)
	case action == "teams" && post && teamName == "":
		s.addGameTeam(w, r, id)
	case action == "teams" && r.Method == http.MethodDelete && teamName != "":
		s.delGameTeam(w, id, teamName)
	case action == "round" && post:
		s.mu.Lock()
		s.ensureSessionLocked(g)
		msg := s.openRoundLocked(id)
		sess := *s.sess(id)
		s.mu.Unlock()
		if msg != "" {
			http.Error(w, msg, 409)
			return
		}
		writeJSON(w, sess)
	case action == "reset" && post:
		s.mu.Lock()
		s.resetLocked(id)
		s.mu.Unlock()
		writeJSON(w, map[string]any{"ok": true})
	case action == "session" && post:
		var body sessionBody
		json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.ensureSessionLocked(g)
		s.applySessionLocked(id, body)
		sess := *s.sess(id)
		s.mu.Unlock()
		writeJSON(w, sess)
	case action == "test" && post:
		s.mu.Lock()
		s.ensureSessionLocked(g)
		s.mu.Unlock()
		now := time.Now().Unix()
		exp := now + 4*3600
		if g.ExpiresAt != 0 && g.ExpiresAt < exp {
			exp = g.ExpiresAt
		}
		// a distinct, non-team sub so this never collides with a real roster
		// entry, carrying only "tester" (round-gate bypass) — not real admin.
		tok := auth.SignTokenGame(s.store.Secret, "__test__:"+c.Sub, []string{"tester"}, id, exp)
		writeJSON(w, map[string]any{"token": tok})
	case action == "board" && get:
		s.mu.Lock()
		st := s.standings(id)
		s.mu.Unlock()
		writeJSON(w, st)
	case action == "leaderboard" && get:
		s.mu.Lock()
		lb := s.leaderboardLocked(id)
		s.mu.Unlock()
		writeJSON(w, lb)
	default:
		methodNotAllowed(w, r)
	}
}

// editGame updates a game's facilitator-editable settings (name, rounds, AP,
// timer, scenario). Scenario only affects teams that start play afterward.
func (s *server) editGame(w http.ResponseWriter, r *http.Request, g *db.GameRow) {
	var b struct {
		Name      string `json:"name"`
		Rounds    int    `json:"rounds"`
		Ap        int    `json:"ap"`
		TimerSecs int    `json:"timerSecs"`
		Scenario  string `json:"scenario"`
	}
	json.NewDecoder(r.Body).Decode(&b)
	name := strings.TrimSpace(b.Name)
	if name == "" {
		http.Error(w, "enter a game name", 400)
		return
	}
	if taken, _ := s.db.GameNameTaken(g.Owner, name, g.ID); taken {
		http.Error(w, "you already have a game with that name", 409)
		return
	}
	g.Name = name
	if b.Rounds > 0 {
		g.Rounds = clampInt(b.Rounds, 1, 8)
	}
	if b.Ap > 0 {
		g.Ap = clampInt(b.Ap, 2, 6)
	}
	if b.TimerSecs > 0 {
		g.TimerSecs = clampInt(b.TimerSecs, 30, 3600)
	}
	if sc := strings.TrimSpace(b.Scenario); sc != "" {
		g.Scenario = sc
	}
	if err := s.db.UpdateGameMeta(g); err != nil {
		s.logger().Error("request failed", "path", r.URL.Path, "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	// reflect new round rules in the live session (without disturbing run state)
	s.mu.Lock()
	if sess := s.sessions[g.ID]; sess != nil {
		sess.Rounds, sess.Ap, sess.TimerSecs = g.Rounds, g.Ap, g.TimerSecs
	}
	s.saveState()
	s.mu.Unlock()
	writeJSON(w, g)
}

// ensureSessionLocked initializes a game's runtime session from its row settings
// if not already present. Caller holds s.mu.
func (s *server) ensureSessionLocked(g *db.GameRow) {
	if s.sessions[g.ID] == nil {
		s.sessions[g.ID] = &gameSession{Rounds: g.Rounds, Ap: g.Ap, TimerSecs: g.TimerSecs}
	}
}

// handleGameJoin (public): a team joins a game with its code + a chosen name,
// receiving a game-scoped token — no account needed.
func (s *server) handleGameJoin(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "multi-game requires the database", 503)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	var b struct{ Code, Team string }
	json.NewDecoder(r.Body).Decode(&b)
	code := strings.ToUpper(strings.TrimSpace(b.Code))
	team := strings.TrimSpace(b.Team)
	if code == "" {
		http.Error(w, "enter your game code", 400)
		return
	}
	var gameRow *db.GameRow
	name := team
	if rt, _ := s.db.GameTeamByCode(code); rt != nil {
		gameRow, _ = s.db.GetGame(rt.GameID) // per-team roster code → join as that team
		name = rt.Name
	} else if g, _ := s.db.GetGameByCode(code); g != nil {
		if name == "" { // game-level code → self-serve; a team name is required
			http.Error(w, "enter a team name", 400)
			return
		}
		gameRow = g
	}
	if gameRow == nil {
		http.Error(w, "no game with that code", 404)
		return
	}
	now := time.Now().Unix()
	if gameRow.ExpiresAt != 0 && now >= gameRow.ExpiresAt {
		http.Error(w, "that game has ended", 410)
		return
	}
	exp := now + 12*3600
	if gameRow.ExpiresAt != 0 && gameRow.ExpiresAt < exp {
		exp = gameRow.ExpiresAt
	}
	tok := auth.SignTokenGame(s.store.Secret, name, []string{"player"}, gameRow.ID, exp)
	writeJSON(w, map[string]any{"token": tok, "gameId": gameRow.ID, "team": name, "name": gameRow.Name})
}
