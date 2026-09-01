package main

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"net/http"
	"strings"

	"conway/server/auth"
	"conway/server/db"
)

// baselineSnapshotID is the snapshot seeded once from server/db/seed/baseline.sql
// (see db.SeedBaseline) — the initial org capture both Observe and Train use
// until a manager imports a real one from Jira.
const baselineSnapshotID = "baseline"

// worldDocs are the snapshot documents the game engine needs to build a World.
var worldDocs = []string{"pods.json", "pod_stats.json", "edges.json", "hygiene.json"}

// handleSnapshots lists snapshots (metadata only). The baseline is always
// visible; otherwise admins see all, owners see their own.
func (s *server) handleSnapshots(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "snapshots require the database", 503)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r)
		return
	}
	rows, err := s.db.ListSnapshots(c.Sub, c.Has("admin"))
	if err != nil {
		s.logger().Error().Str("path", r.URL.Path).Err(err).Msg("request failed")
		http.Error(w, err.Error(), 500)
		return
	}
	type dto struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Source    string          `json:"source"`
		Owner     string          `json:"owner"`
		Mine      bool            `json:"mine"` // caller owns it (can manage/publish)
		Public    bool            `json:"public"`
		RosterID  string          `json:"rosterId,omitempty"`
		Scope     json.RawMessage `json:"scope,omitempty"`
		DocCount  int             `json:"docCount"`
		CreatedAt int64           `json:"createdAt"`
	}
	out := make([]dto, 0, len(rows))
	for _, rr := range rows {
		out = append(out, dto{ID: rr.ID, Name: rr.Name, Source: rr.Source,
			Owner: rr.Owner, Mine: rr.Owner == c.Sub || c.Has("admin"), Public: rr.Public,
			RosterID: rr.RosterID, Scope: json.RawMessage(rr.Scope), DocCount: rr.DocCount, CreatedAt: rr.CreatedAt})
	}
	writeJSON(w, out)
}

// handleSnapshotItem serves a snapshot's documents:
//
//	GET /api/snapshots/{id}/data/{path}  -> the stored JSON blob
//
// This is the single read path that replaces Observe's static data/ fetches.
func (s *server) handleSnapshotItem(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "snapshots require the database", 503)
		return
	}
	id, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/snapshots/"), "/")
	action, docPath, _ := strings.Cut(rest, "/")
	if id == "" {
		http.Error(w, "no snapshot id", 400)
		return
	}
	switch {
	case action == "data" && docPath != "" && r.Method == http.MethodGet:
		body, ok := s.tableDoc(id, docPath)
		if !ok {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	case action == "wip" && r.Method == http.MethodGet:
		s.handleWip(w, r, id)
	case action == "fever" && r.Method == http.MethodGet:
		s.handleFever(w, r, id)
	case action == "hygiene-issues" && r.Method == http.MethodGet:
		s.handleHygieneIssues(w, r, id)
	case action == "edge-issues" && r.Method == http.MethodGet:
		s.handleEdgeIssues(w, r, id)
	case action == "unassoc-epics" && r.Method == http.MethodGet:
		s.handleUnassocEpics(w, id)
	case action == "epic-stats" && r.Method == http.MethodGet:
		s.handleEpicStats(w, id)
	case action == "wip-summary" && r.Method == http.MethodGet:
		s.handleWipSummary(w, id)
	case action == "epic" && docPath != "" && r.Method == http.MethodGet:
		s.handleEpicView(w, id, docPath)
	case action == "export" && r.Method == http.MethodGet:
		s.handleSnapshotExport(w, id, c)
	case action == "clone" && r.Method == http.MethodPost:
		s.handleSnapshotClone(w, r, id, c)
	case action == "" && r.Method == http.MethodDelete:
		s.deleteSnapshot(w, id, c)
	case action == "" && r.Method == http.MethodPatch:
		s.patchSnapshot(w, r, id, c)
	default:
		methodNotAllowed(w, r)
	}
}

// ownedSnapshot loads a snapshot and enforces that the caller may manage it
// (owner or admin). Returns nil after writing an error response.
func (s *server) ownedSnapshot(w http.ResponseWriter, id string, c auth.Claims) *db.SnapshotRow {
	row, err := s.db.GetSnapshot(id)
	if err != nil {
		s.logger().Error().Err(err).Msg("request failed")
		http.Error(w, err.Error(), 500)
		return nil
	}
	if row == nil {
		http.Error(w, "snapshot not found", 404)
		return nil
	}
	if row.Owner != c.Sub && !c.Has("admin") {
		http.Error(w, "forbidden", 403)
		return nil
	}
	return row
}

func (s *server) deleteSnapshot(w http.ResponseWriter, id string, c auth.Claims) {
	row := s.ownedSnapshot(w, id, c)
	if row == nil {
		return
	}
	if row.Source == "baseline" {
		http.Error(w, "the baseline snapshot can't be deleted", 409)
		return
	}
	if err := s.db.DeleteSnapshot(id); err != nil {
		s.logger().Error().Err(err).Msg("request failed")
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// patchSnapshot updates a snapshot's name and/or visibility (owner or admin).
func (s *server) patchSnapshot(w http.ResponseWriter, r *http.Request, id string, c auth.Claims) {
	row := s.ownedSnapshot(w, id, c)
	if row == nil {
		return
	}
	var b struct {
		Name     *string `json:"name"`
		Public   *bool   `json:"public"`
		RosterID *string `json:"rosterId"` // re-associate: rebuild structure from this roster
	}
	json.NewDecoder(r.Body).Decode(&b)
	if b.RosterID != nil {
		pods, err := s.rosterPods(*b.RosterID)
		if err != nil {
			s.logger().Error().Str("path", r.URL.Path).Err(err).Msg("request failed")
			http.Error(w, err.Error(), 500)
			return
		}
		if pods == nil {
			http.Error(w, "roster not found", 400)
			return
		}
		// rebuild the pod structure from the roster; keep the activity
		// (issues/stats/edges/hygiene). Table-backed snapshots rewrite the pods
		// table; legacy blob snapshots rewrite the pods.json doc.
		if existing, _ := s.db.Pods(id); len(existing) > 0 {
			rows := make([]db.PodRow, len(pods))
			for i, p := range pods {
				rows[i] = db.PodRow{Name: p.Name, Location: p.Location, Pairing: p.Pairing, DevCount: p.DevCount, Streams: p.Streams}
			}
			if err := s.db.SetSnapshotPods(id, rows); err != nil {
				s.logger().Error().Str("path", r.URL.Path).Err(err).Msg("request failed")
				http.Error(w, err.Error(), 500)
				return
			}
		} else if err := s.db.PutSnapshotDoc(id, "pods.json", podsAndOverlapDoc(pods)); err != nil {
			s.logger().Error().Str("path", r.URL.Path).Err(err).Msg("request failed")
			http.Error(w, err.Error(), 500)
			return
		}
		if err := s.db.SetSnapshotRoster(id, *b.RosterID); err != nil {
			s.logger().Error().Str("path", r.URL.Path).Err(err).Msg("request failed")
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if b.Name != nil {
		name := strings.TrimSpace(*b.Name)
		if name == "" {
			http.Error(w, "enter a name", 400)
			return
		}
		if err := s.db.RenameSnapshot(id, name); err != nil {
			s.logger().Error().Str("path", r.URL.Path).Err(err).Msg("request failed")
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if b.Public != nil {
		if err := s.db.SetSnapshotPublic(id, *b.Public); err != nil {
			s.logger().Error().Str("path", r.URL.Path).Err(err).Msg("request failed")
			http.Error(w, err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

// worldFromSnapshot builds a game World from a stored snapshot's table rows.
func (s *server) worldFromSnapshot(id string) (*World, error) {
	if s.db == nil {
		return nil, fmt.Errorf("no database")
	}
	docs := map[string][]byte{}
	for _, name := range worldDocs {
		b, ok := s.tableDoc(id, name)
		if !ok {
			return nil, fmt.Errorf("snapshot %s has no %s", id, name)
		}
		docs[name] = b
	}
	return buildWorld(func(name string, v any) error {
		b, ok := docs[name]
		if !ok {
			return fmt.Errorf("snapshot doc %s not found", name)
		}
		return json.Unmarshal(b, v)
	})
}

// defaultWorld resolves the world backing the default-game path and plain
// difficulty-preset scenarios (games seeded from a specific snapshot or plan
// resolve their own world directly and never call this). Driven by whatever
// data actually exists, not by whether it came from the seed: prefers the
// snapshot literally named "baseline", else falls back to the most recently
// created snapshot of any source. Returns nil if the DB has no snapshots at
// all yet — every caller already handles a nil world as "this path isn't
// available."
func (s *server) defaultWorld() *World {
	if w, err := s.worldFromSnapshot(baselineSnapshotID); err == nil {
		return w
	}
	rows, err := s.db.ListSnapshots("", true)
	if err != nil || len(rows) == 0 {
		log.Warn().Msg("no snapshot available yet to source the default world from (expected on a fresh, unseeded DB before any import)")
		return nil
	}
	w, err := s.worldFromSnapshot(rows[0].ID)
	if err != nil {
		log.Error().Str("snapshot", rows[0].ID).Err(err).Msg("could not build the default world from the most recent snapshot")
		return nil
	}
	log.Info().Str("wanted", baselineSnapshotID).Str("using", rows[0].ID).Msg("named snapshot missing; using the most recent as the default world")
	return w
}
