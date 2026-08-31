package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"fmt"
	"math"
)

// Rosters are reusable, editable team-structure definitions (pods). They feed
// Jira imports and can be edited independently. Manager-owned.

func (s *server) handleRosters(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "rosters require the database", 503)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.ListRosters(c.Sub, c.Has("admin"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		type dto struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Owner     string `json:"owner"`
			Mine      bool   `json:"mine"`
			Public    bool   `json:"public"`
			PodCount  int    `json:"podCount"`
			UpdatedAt int64  `json:"updatedAt"`
		}
		out := make([]dto, 0, len(rows))
		for _, rr := range rows {
			out = append(out, dto{ID: rr.ID, Name: rr.Name, Owner: rr.Owner,
				Mine: rr.Owner == c.Sub || c.Has("admin"), Public: rr.Public, PodCount: rr.PodCount, UpdatedAt: rr.UpdatedAt})
		}
		writeJSON(w, out)
	case http.MethodPost:
		var b struct {
			Name string   `json:"name"`
			Pods []NetPod `json:"pods"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		name := strings.TrimSpace(b.Name)
		if name == "" {
			http.Error(w, "name the roster", 400)
			return
		}
		if err := validLosses(b.Pods); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		pods, _ := json.Marshal(b.Pods)
		id := newID()
		if err := s.db.CreateRoster(db.RosterRow{ID: id, Owner: c.Sub, Name: name, Pods: pods, CreatedAt: time.Now().Unix()}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"id": id, "name": name, "pods": len(b.Pods)})
	default:
		methodNotAllowed(w, r)
	}
}

func (s *server) handleRosterItem(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "rosters require the database", 503)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/rosters/")
	if id == "" {
		http.Error(w, "no roster id", 400)
		return
	}
	row, err := s.db.GetRoster(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if row == nil {
		http.Error(w, "roster not found", 404)
		return
	}
	owned := row.Owner == c.Sub || c.Has("admin")
	// GET allowed for owner/admin or a public (shared) roster; edits owner/admin only.
	if !owned && !(r.Method == http.MethodGet && row.Public) {
		http.Error(w, "forbidden", 403)
		return
	}
	switch r.Method {
	case http.MethodGet:
		var pods []NetPod
		if len(row.Pods) > 0 {
			json.Unmarshal(row.Pods, &pods)
		}
		writeJSON(w, map[string]any{"id": row.ID, "name": row.Name, "pods": pods,
			"public": row.Public, "mine": owned, "updatedAt": row.UpdatedAt})
	case http.MethodPatch:
		var b struct {
			Name   string   `json:"name"`
			Pods   []NetPod `json:"pods"`
			Public *bool    `json:"public"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		if b.Public != nil { // visibility-only toggle
			if err := s.db.SetRosterPublic(id, *b.Public); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		name := strings.TrimSpace(b.Name)
		if name == "" {
			name = row.Name
		}
		if err := validLosses(b.Pods); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		pods, _ := json.Marshal(b.Pods)
		if err := s.db.UpdateRoster(id, name, pods, time.Now().Unix()); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "pods": len(b.Pods)})
	case http.MethodDelete:
		if err := s.db.DeleteRoster(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		methodNotAllowed(w, r)
	}
}

// rosterPods loads a roster's pods (structure) for use by an import or a
// re-association. Returns (nil,nil) when id is empty.
func (s *server) rosterPods(id string) ([]NetPod, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	row, err := s.db.GetRoster(id)
	if err != nil || row == nil {
		return nil, err
	}
	var pods []NetPod
	if len(row.Pods) > 0 {
		json.Unmarshal(row.Pods, &pods)
	}
	return pods, nil
}

// validLosses enforces spec 014 FR-004 on roster-authored pods: a capacity
// loss override must be a fraction in [0, 1). The file-upload path refuses
// bad values naming the pod; the roster API is held to the same contract, so
// a crafted loss cannot reach the engine and split its arithmetic in two.
func validLosses(pods []NetPod) error {
	for _, p := range pods {
		if p.CapacityLoss < 0 || p.CapacityLoss >= 1 || math.IsNaN(p.CapacityLoss) {
			return fmt.Errorf("pod %q: capacity loss must be a fraction between 0 and 1", p.Name)
		}
	}
	return nil
}
