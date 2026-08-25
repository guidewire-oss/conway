// Conway game server: serves the static app and provides admin-managed,
// expiring team logins plus a state drop for the live admin console.
//
//	go run .            (from server/)  -> http://localhost:8741
//
// Admin password: $CONWAY_ADMIN_PASSWORD, else generated and printed once.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/game"
	"conway/server/oidc"
)

// Play state is scoped per game (gameID -> team -> …). The legacy single game is
// just the game with id defaultGameID; multiple facilitator-owned games coexist.
const defaultGameID = "default"

type server struct {
	mu        sync.Mutex
	store     *auth.Store
	teams     map[string]map[string]json.RawMessage // gameID -> team -> standings
	games     map[string]map[string]*game.Game      // gameID -> team -> authoritative game
	sessions  map[string]*gameSession               // gameID -> session (run state)
	world     *World
	appDir    string
	db        *db.DB // Postgres backend (nil = file/in-memory mode for local dev)
	statePath string // file path the play state is snapshotted to when db is nil

	jiraOAuth    *jiraOAuthConfig        // nil = OAuth not configured (token path only)
	jiraMu       sync.Mutex              // guards jiraSessions
	jiraSessions map[string]*jiraSession // conway user sub -> connected Jira OAuth session
	jiraBaseURL  string                  // e.g. https://yourorg.atlassian.net — for browse links in the UI
	jiraSiteHint string                  // substring preferred among accessible OAuth sites (e.g. your org's Jira hostname)

	importMu   sync.Mutex            // guards importJobs
	importJobs map[string]*importJob // async Jira-import jobs by id

	oidc      *oidc.Provider       // nil = SSO not configured (password login only)
	oidcMu    sync.Mutex           // guards oidcFlows
	oidcFlows map[string]*oidcFlow // pending SSO logins keyed by state (PKCE + nonce)
}

// sess/gmap/tmap return the per-game maps, creating them on first use. Caller holds s.mu.
func (s *server) sess(gid string) *gameSession {
	if s.sessions[gid] == nil {
		s.sessions[gid] = &gameSession{Rounds: 4, Ap: 5, TimerSecs: 300}
	}
	return s.sessions[gid]
}
func (s *server) gmap(gid string) map[string]*game.Game {
	if s.games[gid] == nil {
		s.games[gid] = map[string]*game.Game{}
	}
	return s.games[gid]
}
func (s *server) tmap(gid string) map[string]json.RawMessage {
	if s.teams[gid] == nil {
		s.teams[gid] = map[string]json.RawMessage{}
	}
	return s.teams[gid]
}

// gameID resolves which game a request acts on: a join token's scope, else the
// default game (legacy account players + the legacy admin endpoints).
func gameID(c auth.Claims) string {
	if c.GameID != "" {
		return c.GameID
	}
	return defaultGameID
}

// persisted is the snapshot written to the PVC so a pod replacement (redeploy,
// eviction, node drain) resumes the live game instead of resetting it.
type persisted struct {
	Sessions map[string]*gameSession               `json:"sessions"`
	Games    map[string]map[string]*game.Game      `json:"games"`
	Teams    map[string]map[string]json.RawMessage `json:"teams"`
}

// saveState snapshots the in-memory game (Postgres when configured, else an
// atomic file write). Caller holds s.mu.
func (s *server) saveState() {
	b, err := json.Marshal(persisted{Sessions: s.sessions, Games: s.games, Teams: s.teams})
	if err != nil {
		log.Printf("saveState marshal: %v", err)
		return
	}
	if s.db != nil {
		if err := s.db.SaveGameState(b); err != nil {
			log.Printf("saveState db: %v", err)
		}
		return
	}
	if s.statePath == "" {
		return
	}
	tmp := s.statePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		log.Printf("saveState write: %v", err)
		return
	}
	if err := os.Rename(tmp, s.statePath); err != nil {
		log.Printf("saveState rename: %v", err)
	}
}

// loadState restores a snapshot on boot (called before serving). Re-arms the
// round timer if a round was open with a deadline still in the future.
func (s *server) loadState() {
	var b []byte
	if s.db != nil {
		got, err := s.db.LoadGameState()
		if err != nil {
			log.Printf("loadState db: %v", err)
			return
		}
		if got == nil {
			// one-time import of a legacy file snapshot into the DB
			if s.statePath != "" {
				if fb, e := os.ReadFile(s.statePath); e == nil {
					got = fb
				}
			}
			if got == nil {
				return
			} // nothing saved yet
			defer s.saveState() // persist the imported snapshot into Postgres
		}
		b = got
	} else {
		if s.statePath == "" {
			return
		}
		got, err := os.ReadFile(s.statePath)
		if err != nil {
			return
		} // no prior snapshot — fresh start
		b = got
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		log.Printf("loadState: %v", err)
		return
	}
	if p.Sessions != nil {
		s.sessions = p.Sessions
	}
	if p.Games != nil {
		s.games = p.Games
	}
	if p.Teams != nil {
		s.teams = p.Teams
	}
	if s.sessions[defaultGameID] == nil {
		s.sessions[defaultGameID] = &gameSession{Rounds: 4, Ap: 5, TimerSecs: 300}
	}
	// every game in the registry needs a session even if it predates the snapshot
	if s.db != nil {
		if rows, err := s.db.ListGames("", true); err == nil {
			for _, gr := range rows {
				if s.sessions[gr.ID] == nil {
					s.sessions[gr.ID] = &gameSession{Rounds: gr.Rounds, Ap: gr.Ap, TimerSecs: gr.TimerSecs}
				}
			}
		}
	}
	games := 0
	for _, m := range s.games {
		games += len(m)
	}
	log.Printf("restored game state: %d game(s), %d team-games", len(s.sessions), games)
	// re-arm each open game's timer (fires immediately if the deadline already lapsed)
	for gid, sess := range s.sessions {
		if sess.OpenRound >= 1 && sess.Deadline > 0 {
			s.armAutoSubmit(gid, sess.OpenRound, sess.Deadline)
		}
	}
}

// gameSession is the single set of game parameters the facilitator opens for the
// whole room, so every team plays the same rounds/AP and scores are comparable.
type gameSession struct {
	Rounds    int   `json:"rounds"`
	Ap        int   `json:"ap"`
	Open      bool  `json:"open"`
	OpenRound int   `json:"openRound"` // highest round opened for play (0 = none)
	TimerSecs int   `json:"timerSecs"` // per-round countdown; 0 = no timer
	Deadline  int64 `json:"deadline"`  // unix secs the current open round auto-submits (0 = none)
}

// noCache makes browsers revalidate static assets on every load, so a redeploy
// is picked up immediately instead of serving a stale cached main.js/style.css.
// Last-Modified still yields cheap 304s when nothing changed.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

// allowedRoles filters a requested role set down to the known roles, de-duped.
func allowedRoles(in []string) []string {
	known := map[string]bool{"admin": true, "facilitator": true, "manager": true, "player": true}
	seen := map[string]bool{}
	var out []string
	for _, r := range in {
		if known[r] && !seen[r] {
			out = append(out, r)
			seen[r] = true
		}
	}
	return out
}

// loadLegacyStore reads a pre-Postgres JSON file store, if one exists, so its
// accounts can be imported into the DB on first boot. Returns nil if absent.
func loadLegacyStore(path string) *auth.Store {
	ls := auth.NewStore(nil)
	ls.Path = path
	if err := ls.Load(); err != nil {
		return nil
	}
	return ls
}

// What to do with the admin account on boot.
const (
	adminNothing        = ""                 // nothing to do
	adminGenerate       = "generate"         // no admin, no variable: mint one and print it
	adminSetFromEnv     = "set-from-env"     // no admin: take the variable
	adminReplaceFromEnv = "replace-from-env" // admin exists with a different password
)

// adminAction decides how CONWAY_ADMIN_PASSWORD applies this boot.
//
// The variable wins whenever it is set, because it is the only way to set this
// password — nothing in the app changes it — so there is no in-app value for a
// stale variable to clobber, and gating it to a first boot made it silently inert
// on every restart afterwards.
//
// It is not applied when it already matches, so a deployment that leaves the
// variable set does not rewrite its own hash on every restart, and the log line
// means something when it does appear.
func adminAction(st *auth.Store, envPw string) string {
	u := st.Users["admin"]
	switch {
	case envPw == "" && u == nil:
		return adminGenerate
	case envPw == "":
		return adminNothing
	case u == nil:
		return adminSetFromEnv
	case auth.VerifyPassword(envPw, u.Salt, u.Hash):
		return adminNothing // already this password; leave the stored hash alone
	default:
		return adminReplaceFromEnv
	}
}

// retireLegacyStore renames the legacy file store aside once its contents are in
// Postgres. Renamed rather than deleted: it holds credentials and a signing
// secret, and an operator who wants them back should not have to reach for a
// backup.
func retireLegacyStore(path string) {
	retired := path + ".migrated"
	// Refuse to overwrite an earlier backup. os.Rename would replace it silently on
	// Unix, and this file holds credentials and a signing secret — losing an older
	// copy to a convenience rename is not a trade worth making. Lstat, not Stat, so
	// a dangling symlink counts as present rather than as a free slot.
	if _, err := os.Lstat(retired); err == nil {
		log.Printf("not retiring legacy store %s: %s already exists. "+
			"Move it aside if you want the import retired, or delete %s to stop it being re-imported",
			path, retired, path)
		return
	}
	if err := os.Rename(path, retired); err != nil {
		// Not fatal. The accounts are already saved; the cost is that a later reset
		// will import them again, which is the behaviour this replaces rather than a
		// new failure. Say so instead of stopping the server.
		log.Printf("could not retire legacy store %s (%v) — delete it by hand, "+
			"or an admin reset will restore these accounts again", path, err)
		return
	}
	log.Printf("retired legacy store to %s — it will not be imported again", retired)
}

// ensureStoreDir creates the directory holding the legacy file store, and reports
// failure rather than ending the process.
//
// It used to be fatal, which took every pod on the dev cluster down: the image
// bakes CONWAY_STORE=/data/store.json and the container runs with a read-only root
// filesystem, so mkdir failed and the server exited over a directory it never
// writes to. Nothing is written here while a database is configured, and one is
// mandatory.
func ensureStoreDir(storePath string) error {
	dir := filepath.Dir(storePath)
	if storePath == "" || dir == "" || dir == "." {
		return nil
	}
	// 0o750, not 0o755: this held the credential store and game state, so there is
	// no reason for it to be world-readable (gosec G301).
	return os.MkdirAll(dir, 0o750)
}

func main() {
	// Defaults assume the repo root as the working directory (the module root
	// since go.mod moved there): the SPA is ./app, and everything written at
	// runtime — this store plus the game state beside it — goes under ./var.
	addr := env("CONWAY_ADDR", ":8741")
	appDir := env("CONWAY_APP_DIR", "./app")
	storePath := env("CONWAY_STORE", "./var/store.json")
	// The store directory is only ever read now, by the one-time legacy imports
	// below: DATABASE_URL is required a few lines down and Postgres is the only
	// backend, so accounts, the signing secret and the game snapshot are all in
	// the database. Not being able to create it is therefore a note, not a reason
	// to exit -- a read-only root filesystem is a perfectly good way to run this.
	if err := ensureStoreDir(storePath); err != nil {
		log.Printf("store directory unavailable (%v); continuing, since Postgres holds "+
			"the accounts, the signing secret and the game state", err)
	}

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required (Postgres is the only supported backend — see docker-compose.yml)")
	}
	st := auth.NewStore(nil)
	database, err := db.Open(context.Background(), url)
	if err != nil {
		log.Fatalf("connect Postgres: %v", err)
	}
	st.SetBackend(database)
	log.Printf("persistence: Postgres")
	_ = st.Load() // a missing/empty store just means a fresh start
	// One-time import of a legacy file store into the DB (preserves accounts +
	// signing secret from a pre-Postgres deployment, if one exists at storePath).
	//
	// It fires whenever the accounts table is empty, which is not the same as "the
	// first ever boot": clearing accounts deliberately leaves the table empty too,
	// and the import then quietly restores what was just removed. So the file is
	// retired once its contents are safely in Postgres, which is what makes
	// "one-time" true rather than aspirational.
	legacyImported := ""
	if len(st.Users) == 0 {
		if legacy := loadLegacyStore(storePath); legacy != nil && len(legacy.Users) > 0 {
			if len(legacy.Secret) > 0 {
				st.Secret = legacy.Secret
			}
			st.Users = legacy.Users
			legacyImported = storePath
			log.Printf("migrated %d account(s) from legacy %s", len(st.Users), storePath)
		}
	}
	if len(st.Secret) == 0 {
		secret := make([]byte, 32)
		if _, e := randRead(secret); e != nil {
			log.Fatal(e)
		}
		st.Secret = secret
	}
	// Ensure an admin exists, and let CONWAY_ADMIN_PASSWORD win every boot.
	//
	// It used to apply only when no admin existed, which made it silently inert on
	// every boot after the first: the only symptom was the sign-in refusing the
	// password, and the only recovery was deleting the row by hand. It is also the
	// *only* way to set this password — nothing in the app changes it — so there is
	// no in-app value for a stale variable to clobber, and the secret already lives
	// in the environment either way. Gating it bought no privacy and cost a trap.
	switch envPw := os.Getenv("CONWAY_ADMIN_PASSWORD"); adminAction(st, envPw) {
	case adminGenerate:
		pw := randPw()
		log.Printf("=== Conway admin password (save this): %s ===", pw)
		st.SetAdmin(pw)
	case adminSetFromEnv:
		st.SetAdmin(envPw)
		// Never silent in either direction: silence is what made the old behaviour so
		// hard to diagnose. The value itself is not logged.
		log.Printf("admin password set from CONWAY_ADMIN_PASSWORD")
	case adminReplaceFromEnv:
		st.SetAdmin(envPw)
		log.Printf("admin password set from CONWAY_ADMIN_PASSWORD (replaced the existing one)")
	}
	must(st.Save())
	// Only after the accounts are durably in Postgres, so a failed Save leaves the
	// legacy file exactly where it was.
	if legacyImported != "" {
		retireLegacyStore(legacyImported)
	}

	s := &server{store: st,
		teams:    map[string]map[string]json.RawMessage{},
		games:    map[string]map[string]*game.Game{},
		sessions: map[string]*gameSession{},
		appDir:   appDir, db: database,
		statePath:    filepath.Join(filepath.Dir(storePath), "game-state.json"),
		jiraSessions: map[string]*jiraSession{},
		importJobs:   map[string]*importJob{},
		jiraBaseURL:  strings.TrimRight(os.Getenv("CONWAY_JIRA_BASE_URL"), "/"),
		jiraSiteHint: os.Getenv("CONWAY_JIRA_SITE_HINT"),
	}
	if id := os.Getenv("CONWAY_JIRA_CLIENT_ID"); id != "" {
		s.jiraOAuth = &jiraOAuthConfig{
			ClientID:     id,
			ClientSecret: os.Getenv("CONWAY_JIRA_CLIENT_SECRET"),
			PublicURL:    env("CONWAY_PUBLIC_URL", "http://localhost:8741"),
		}
		log.Printf("Jira OAuth configured (redirect %s/api/jira/oauth/callback)", s.jiraOAuth.PublicURL)
	}
	s.oidcFlows = map[string]*oidcFlow{}
	s.oidc = buildOIDC(context.Background(), env("CONWAY_PUBLIC_URL", "http://localhost:8741"))
	// CONWAY_SEED_BASELINE=false skips writing the demo dataset; either way,
	// the default world (see defaultWorld below) is resolved the same way
	// from whatever's actually in the DB — seeded or real, never special-cased.
	if env("CONWAY_SEED_BASELINE", "true") != "false" {
		if err := database.SeedBaseline(context.Background()); err != nil {
			log.Printf("warning: could not seed baseline snapshot: %v", err)
		}
	}
	s.world = s.defaultWorld()
	s.sessions[defaultGameID] = &gameSession{Rounds: 4, Ap: 5, TimerSecs: 300}
	s.loadState() // resume live games across pod replacement (redeploy / eviction)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		sess := *s.sess(defaultGameID)
		s.mu.Unlock()
		// serverGame reflects whether the Train pillar can work at all (the DB is
		// mandatory, so: always) — NOT whether s.world (the default-game/
		// difficulty-preset world) happens to be loaded. A game seeded from a
		// specific snapshot (the normal path) never touches s.world.
		writeJSON(w, map[string]any{"server": true, "authRequired": true, "serverGame": true,
			"gameOpen": sess.Open, "rounds": sess.Rounds, "ap": sess.Ap,
			"openRound": sess.OpenRound, "timerSecs": sess.TimerSecs, "deadline": sess.Deadline,
			"jiraBaseUrl": s.jiraBaseURL, "oidc": s.oidcEnabled()})
	})
	mux.HandleFunc("/api/login", s.handleLogin)
	// SSO (OIDC) — both public: start returns the authorize URL, callback is the
	// browser redirect target identified by the signed state (no bearer header).
	mux.HandleFunc("/api/oidc/start", s.handleOIDCStart)
	mux.HandleFunc("/api/oidc/callback", s.handleOIDCCallback)
	mux.HandleFunc("/api/me", s.withAuth(s.handleMe, ""))
	// account & role management is admin-only; running the game is facilitator
	// (admin passes too, as a superuser).
	mux.HandleFunc("/api/admin/users", s.withAuth(s.handleUsers, "admin"))
	mux.HandleFunc("/api/admin/users/", s.withAuth(s.handleUserItem, "admin"))
	mux.HandleFunc("/api/admin/board", s.withAuth(s.handleBoard, "facilitator"))
	mux.HandleFunc("/api/admin/game", s.withAuth(s.handleAdminGame, "facilitator"))
	mux.HandleFunc("/api/admin/session", s.withAuth(s.handleSession, "facilitator"))
	mux.HandleFunc("/api/admin/round", s.withAuth(s.handleOpenRound, "facilitator"))
	mux.HandleFunc("/api/admin/reset", s.withAuth(s.handleReset, "facilitator"))
	mux.HandleFunc("/api/leaderboard", s.withAuth(s.handleLeaderboard, "facilitator")) // facilitator/projector only — teams can't pull standings
	// Train pillar — multi-game (facilitator-owned; admin sees all)
	mux.HandleFunc("/api/games/join", s.handleGameJoin) // public: team joins by code
	mux.HandleFunc("/api/games", s.withAuth(s.handleGames, "facilitator"))
	mux.HandleFunc("/api/games/", s.withAuth(s.handleGameItem, "facilitator"))
	// Plan pillar (manager-owned; admin sees all)
	mux.HandleFunc("/api/sample/initiatives.xlsx", s.handleSampleInitiatives) // public sample downloads
	mux.HandleFunc("/api/sample/teams.csv", s.handleSampleTeams)
	mux.HandleFunc("/api/sample/network.json", s.handleSampleNetwork)         // editable scenario format example
	mux.HandleFunc("/api/sample/roster.csv", s.handleSampleRoster)            // team-structure roster example
	mux.HandleFunc("/api/plan/demo", s.withAuth(s.handlePlanDemo, "manager")) // exact match wins over /api/plan/
	mux.HandleFunc("/api/plan", s.withAuth(s.handlePlans, "manager"))
	mux.HandleFunc("/api/plan/", s.withAuth(s.handlePlanItem, "manager"))
	// Snapshots — the org-network captures Observe renders and Train seeds from
	mux.HandleFunc("/api/jira/status", s.withAuth(s.handleJiraStatus, "manager"))
	mux.HandleFunc("/api/jira/oauth/start", s.withAuth(s.handleJiraOAuthStart, "manager"))
	mux.HandleFunc("/api/jira/oauth/callback", s.handleJiraOAuthCallback)             // public: browser redirect, identified by signed state
	mux.HandleFunc("/api/jira/projects", s.withAuth(s.handleJiraProjects, "manager")) // exact match over /api/snapshots/
	mux.HandleFunc("/api/snapshots/import", s.withAuth(s.handleSnapshotImport, "manager"))
	mux.HandleFunc("/api/snapshots/import-status/", s.withAuth(s.handleImportStatus, "manager"))
	mux.HandleFunc("/api/parse-roster", s.withAuth(s.handleParseRoster, "manager")) // CSV/XLSX roster → pods
	mux.HandleFunc("/api/rosters", s.withAuth(s.handleRosters, "manager"))          // saved, editable team rosters
	mux.HandleFunc("/api/rosters/", s.withAuth(s.handleRosterItem, "manager"))
	mux.HandleFunc("/api/snapshots/import-network", s.withAuth(s.handleNetworkImport, "facilitator")) // facilitator templates
	mux.HandleFunc("/api/snapshots", s.withAuth(s.handleSnapshots, ""))
	mux.HandleFunc("/api/snapshots/", s.withAuth(s.handleSnapshotItem, ""))
	mux.HandleFunc("/api/state", s.withAuth(s.handleState, ""))
	// authoritative game (rules live here, never shipped to the client)
	mux.HandleFunc("/api/game/new", s.withAuth(s.handleGameNew, ""))
	mux.HandleFunc("/api/game", s.withAuth(s.handleGameGet, ""))
	mux.HandleFunc("/api/game/config", s.withAuth(s.handleGameConfig, "")) // a joined team's own game session
	mux.HandleFunc("/api/game/stage", s.withAuth(s.handleStage, ""))
	mux.HandleFunc("/api/game/unstage", s.withAuth(s.handleUnstage, ""))
	mux.HandleFunc("/api/game/submit", s.withAuth(s.handleSubmit, ""))
	mux.Handle("/", noCache(http.FileServer(http.Dir(filepath.Clean(appDir)))))

	log.Printf("Conway server on %s serving %s", addr, appDir)
	// An explicit server rather than http.ListenAndServe, so a header-read
	// timeout bounds slowloris-style connections that would otherwise be held
	// open indefinitely. ReadTimeout and WriteTimeout are deliberately unset:
	// uploads run to maxUpload (20MB) over links we do not control, and nothing
	// here streams, so capping whole-request duration would only break large
	// imports. IdleTimeout reaps keep-alive connections instead.
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

// ---- handlers -----------------------------------------------------------

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct{ Username, Password string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.store.Authenticate(strings.TrimSpace(body.Username), body.Password)
	if err != nil {
		http.Error(w, "invalid credentials or expired account", 401)
		return
	}
	writeJSON(w, map[string]any{
		"token": s.store.Token(u), "roles": u.Roles, "display": u.Display, "username": u.Username,
	})
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	writeJSON(w, map[string]any{"username": c.Sub, "roles": c.Roles, "gameId": c.GameID, "exp": c.Exp})
}

func (s *server) handleUsers(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		out := []map[string]any{}
		for _, u := range s.store.Users {
			out = append(out, map[string]any{
				"username": u.Username, "display": u.Display, "roles": u.Roles,
				"expiresAt": u.ExpiresAt, "expired": s.store.Expired(u), "sso": u.SSO,
				"hasState": s.tmap(defaultGameID)[u.Username] != nil,
				"hasGame":  s.gmap(defaultGameID)[u.Username] != nil,
			})
		}
		writeJSON(w, out)
	case http.MethodPost:
		var body struct {
			Display     string   `json:"display"`
			ExpiryHours int      `json:"expiryHours"`
			Roles       []string `json:"roles"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ExpiryHours <= 0 {
			body.ExpiryHours = 48
		}
		roles := allowedRoles(body.Roles) // keep only known roles
		if len(roles) == 0 {
			roles = []string{"player"}
		}
		u, pw := s.store.CreateUser(body.Display, roles, body.ExpiryHours)
		must(s.store.Save())
		writeJSON(w, map[string]any{"username": u.Username, "password": pw, "roles": u.Roles, "expiresAt": u.ExpiresAt})
	default:
		methodNotAllowed(w, r)
	}
}

func (s *server) handleUserItem(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	name := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodDelete:
		if name == "admin" {
			http.Error(w, "the built-in admin account cannot be deleted", 400)
			return
		}
		s.store.Delete(name)
		delete(s.tmap(defaultGameID), name)
		delete(s.gmap(defaultGameID), name)
		s.saveState()
		must(s.store.Save())
		writeJSON(w, map[string]any{"ok": true})
	case r.Method == http.MethodPost && strings.HasSuffix(name, "/extend"):
		name = strings.TrimSuffix(name, "/extend")
		// An admin can push an account out by weeks, not just the hardcoded
		// +24h the button shipped with. A missing body keeps the old default.
		// *int: an explicit {"hours":0} is a request, not a missing field — the
		// default belongs only to bodies that never set it.
		var body struct {
			Hours *int `json:"hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "could not read the extend request: "+err.Error(), 400)
			return
		}
		hours := 24
		if body.Hours != nil {
			if *body.Hours <= 0 {
				http.Error(w, "hours must be positive — to expire a user now, revoke them", 400)
				return
			}
			hours = *body.Hours
		}
		// Extend() adds to the current expiry; a horizon that cannot fit in an
		// int64 second-count would wrap negative and silently expire the user.
		base := s.store.NowUnix()
		if u := s.store.Users[name]; u != nil && u.ExpiresAt > base {
			base = u.ExpiresAt
		}
		if hours < 0 || int64(hours) > (math.MaxInt64-base)/3600 {
			http.Error(w, "hours is outside the supported range — to expire a user now, revoke them", 400)
			return
		}
		if !s.store.Extend(name, hours) {
			http.Error(w, "user not found", 404)
			return
		}
		must(s.store.Save())
		writeJSON(w, map[string]any{"ok": true})
	default:
		methodNotAllowed(w, r)
	}
}

// admin reads or sets the room's game session (rounds, AP, open/closed).
// sessionBody is the editable session payload. Open is a pointer so "Save params"
// (which omits it) changes rounds/ap/timer WITHOUT touching the open/closed state.
type sessionBody struct {
	Rounds    int   `json:"rounds"`
	Ap        int   `json:"ap"`
	TimerSecs int   `json:"timerSecs"`
	Open      *bool `json:"open"`
}

// applySessionLocked updates one game's session params. Caller holds s.mu.
func (s *server) applySessionLocked(gid string, b sessionBody) {
	sess := s.sess(gid)
	if b.Rounds > 0 {
		sess.Rounds = clampInt(b.Rounds, 1, 8)
	}
	if b.Ap > 0 {
		sess.Ap = clampInt(b.Ap, 2, 6)
	}
	if b.TimerSecs > 0 {
		newT := clampInt(b.TimerSecs, 30, 3600)
		changed := newT != sess.TimerSecs
		sess.TimerSecs = newT
		if changed && sess.Open && sess.OpenRound >= 1 {
			sess.Deadline = time.Now().Unix() + int64(newT)
			s.armAutoSubmit(gid, sess.OpenRound, sess.Deadline)
		}
	}
	if b.Open != nil {
		sess.Open = *b.Open
	}
	s.saveState()
}

// handleSession is the legacy default-game session endpoint.
func (s *server) handleSession(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == http.MethodPost {
		var body sessionBody
		json.NewDecoder(r.Body).Decode(&body)
		s.applySessionLocked(defaultGameID, body)
	}
	writeJSON(w, s.sess(defaultGameID))
}

// handleOpenRound opens the next round for play and arms the per-round timer.
// Opening round 1 implies the room is open.
// openRoundLocked opens the next round for a game; returns "" or an error msg.
// Caller holds s.mu.
func (s *server) openRoundLocked(gid string) string {
	sess := s.sess(gid)
	sess.Open = true
	if sess.OpenRound >= sess.Rounds {
		return "all rounds already opened"
	}
	sess.OpenRound++
	if sess.TimerSecs > 0 {
		sess.Deadline = time.Now().Unix() + int64(sess.TimerSecs)
		s.armAutoSubmit(gid, sess.OpenRound, sess.Deadline)
	} else {
		sess.Deadline = 0
	}
	s.saveState()
	return ""
}

func (s *server) handleOpenRound(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if msg := s.openRoundLocked(defaultGameID); msg != "" {
		http.Error(w, msg, 409)
		return
	}
	writeJSON(w, s.sess(defaultGameID))
}

// resetLocked clears a game's play state back to "no round open". Caller holds s.mu.
func (s *server) resetLocked(gid string) {
	s.games[gid] = map[string]*game.Game{}
	s.teams[gid] = map[string]json.RawMessage{}
	sess := s.sess(gid)
	sess.OpenRound = 0
	sess.Deadline = 0
	s.saveState()
}

// armAutoSubmit auto-submits every team still on `round` when the deadline hits.
// A stale timer (round re-opened / deadline changed) is ignored.
func (s *server) armAutoSubmit(gid string, round int, deadline int64) {
	d := time.Until(time.Unix(deadline, 0))
	time.AfterFunc(d, func() {
		// NEVER let a bad round-resolution crash the process — recover, log, keep going.
		defer func() {
			if e := recover(); e != nil {
				// G706 is a false positive once the verb is %q: fmt quotes via
				// strconv.Quote, so a newline in gid renders as \n and cannot forge
				// a log line. Verified: %s emits a second line, %q does not.
				log.Printf("auto-submit %q (round %d) recovered from panic: %v", gid, round, e) //nolint:gosec // G706: %q escapes newlines
			}
		}()
		s.mu.Lock()
		defer s.mu.Unlock()
		sess := s.sess(gid)
		if sess.OpenRound != round || sess.Deadline != deadline {
			return
		} // stale timer
		for team, g := range s.gmap(gid) {
			if u := s.store.Users[team]; u != nil && u.Has("admin") {
				continue
			}
			if g.Round == round && g.Round <= g.TotalRounds {
				func() { // isolate each team so one bad submit can't skip the rest
					defer func() {
						if e := recover(); e != nil {
							log.Printf("auto-submit %q/%q (round %d) recovered: %v", gid, team, round, e) //nolint:gosec // G706: %q escapes newlines, see armAutoSubmit above
						}
					}()
					s.submitRound(gid, g, team)
				}()
			}
		}
		sess.Deadline = 0
		s.saveState()
	})
}

// handleReset clears ALL in-progress games and standings (for a fresh run /
// between groups). Accounts and the session open/closed flag are left intact.
func (s *server) handleReset(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	s.mu.Lock()
	s.resetLocked(defaultGameID)
	s.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// team posts its current game state; admin board reads them all
func (s *server) handleState(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	body, err := readAll(r.Body, 1<<20)
	if err != nil {
		http.Error(w, "too large", 413)
		return
	}
	s.mu.Lock()
	s.tmap(gameID(c))[c.Sub] = body
	s.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

// boardState is the live standing for one team, derived from its authoritative
// in-memory game (so a team shows up the moment it starts, not only after it
// resolves a round).
type boardState struct {
	Round int        `json:"round"`
	Score game.Score `json:"score"`
	Over  bool       `json:"over"`
}

func liveScore(g *game.Game) (game.Score, bool) {
	if g.Round > g.TotalRounds {
		sc, _ := game.FinalScore(g)
		return sc, true
	}
	return scoreView(g), false
}

// caller must hold s.mu
func (s *server) standings(gid string) map[string]boardState {
	out := map[string]boardState{}
	for name, g := range s.gmap(gid) {
		if strings.HasPrefix(name, "__test__:") {
			continue // a facilitator's own 🧪 trial run is not a competitor
		}
		if u := s.store.Users[name]; u != nil && u.Has("admin") {
			continue // the admin's own test game is not a competitor
		}
		sc, over := liveScore(g)
		out[name] = boardState{Round: g.Round, Score: sc, Over: over}
	}
	return out
}

func (s *server) handleBoard(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, s.standings(defaultGameID))
}

type leaderRow struct {
	Team  string     `json:"team"`
	State boardState `json:"state"`
}

// leaderboardLocked builds the projector leaderboard for a game. Caller holds s.mu.
func (s *server) leaderboardLocked(gid string) []leaderRow {
	out := []leaderRow{}
	for name, st := range s.standings(gid) {
		display := name
		if u := s.store.Users[name]; u != nil && u.Display != "" {
			display = u.Display // legacy account teams have a display name; join teams use the name as-is
		}
		out = append(out, leaderRow{Team: display, State: st})
	}
	return out
}

// standings for the projector screen. admin/facilitator-only so a team can't pull
// the board from its own laptop.
func (s *server) handleLeaderboard(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, s.leaderboardLocked(defaultGameID))
}

// ---- middleware & helpers ----------------------------------------------

func (s *server) withAuth(h func(http.ResponseWriter, *http.Request, auth.Claims), role string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		c, err := auth.ParseToken(s.store.Secret, tok, time.Now().Unix())
		if err != nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		if c.GameID != "" {
			// game-scoped join token: validate against the game (which the team
			// joined by code), not an account.
			if s.db != nil {
				g, _ := s.db.GetGame(c.GameID)
				if g == nil || (g.ExpiresAt != 0 && time.Now().Unix() >= g.ExpiresAt) {
					http.Error(w, "that game has ended", 401)
					return
				}
			}
		} else {
			// account-backed token: may have been revoked/expired since issue
			s.mu.Lock()
			u := s.store.Users[c.Sub]
			valid := u != nil && !s.store.Expired(u)
			s.mu.Unlock()
			if !valid {
				http.Error(w, "account revoked or expired", 401)
				return
			}
		}
		// admin is a superuser: it passes every role gate. Otherwise the token
		// must carry the required role.
		if role != "" && !c.Has(role) && !c.Has("admin") {
			http.Error(w, "forbidden", 403)
			return
		}
		h(w, r, c)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
