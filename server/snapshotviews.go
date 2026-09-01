package main

import (
	"net/http"
	"strconv"
	"time"
)

// Table-backed view endpoints (server-side filter/paginate) that replace the
// old epics/wip/hygiene JSON blobs. All read the snapshot_issues tables.

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// GET /api/snapshots/{id}/wip?pod=X&page=N&size=15
func (s *server) handleWip(w http.ResponseWriter, r *http.Request, id string) {
	pod := r.URL.Query().Get("pod")
	size := atoiDefault(r.URL.Query().Get("size"), 15)
	if size < 1 || size > 200 {
		size = 15
	}
	page := atoiDefault(r.URL.Query().Get("page"), 0)
	if page < 0 {
		page = 0
	}
	items, total, freezable, err := s.db.WipPage(id, pod, size, page*size)
	if err != nil {
		s.logger().Error("request failed", "path", r.URL.Path, "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	now := time.Now()
	days := func(t *time.Time) float64 {
		if t == nil {
			return 0
		}
		return now.Sub(*t).Hours() / 24
	}
	type wipOut struct {
		Key        string   `json:"key"`
		Summary    string   `json:"summary"`
		Assignee   string   `json:"assignee,omitempty"`
		AgeDays    float64  `json:"ageDays"`
		StaleDays  float64  `json:"staleDays"`
		BlocksKeys []string `json:"blocksKeys"`
		Verdict    string   `json:"verdict"`
	}
	out := make([]wipOut, 0, len(items))
	for _, it := range items {
		out = append(out, wipOut{it.Key, it.Summary, it.Assignee, days(it.Created), days(it.Updated), it.Blocks, it.Verdict})
	}
	pages := (total + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	writeJSON(w, map[string]any{"items": out, "total": total, "freezable": freezable, "page": page, "pages": pages})
}

// GET /api/snapshots/{id}/fever?n=25
func (s *server) handleFever(w http.ResponseWriter, r *http.Request, id string) {
	n := atoiDefault(r.URL.Query().Get("n"), 25)
	if n < 1 || n > 200 {
		n = 25
	}
	epics, err := s.db.FeverEpics(id, n)
	if err != nil {
		s.logger().Error("request failed", "path", r.URL.Path, "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	total, _ := s.db.EpicCount(id)
	writeJSON(w, map[string]any{"epics": epics, "total": total})
}

// GET /api/snapshots/{id}/hygiene-issues?pod=X
func (s *server) handleHygieneIssues(w http.ResponseWriter, r *http.Request, id string) {
	lists, err := s.db.HygieneIssueLists(id, r.URL.Query().Get("pod"))
	if err != nil {
		s.logger().Error("request failed", "path", r.URL.Path, "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, lists)
}

// GET /api/snapshots/{id}/epic/{key}
func (s *server) handleEpicView(w http.ResponseWriter, id, key string) {
	e, err := s.db.EpicWithTasks(id, key)
	if err != nil {
		http.Error(w, "epic not found", 404)
		return
	}
	writeJSON(w, e)
}

// GET /api/snapshots/{id}/wip-summary
func (s *server) handleWipSummary(w http.ResponseWriter, id string) {
	total, freezable, err := s.db.WipSummary(id)
	if err != nil {
		s.logger().Error("request failed", "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]int{"total": total, "freezable": freezable})
}

// GET /api/snapshots/{id}/epic-stats
func (s *server) handleEpicStats(w http.ResponseWriter, id string) {
	known, missing, overdue, noDue, err := s.db.EpicStats(id)
	if err != nil {
		s.logger().Error("request failed", "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]int{"known": known, "missing": missing, "overdue": overdue, "noDue": noDue})
}

// GET /api/snapshots/{id}/edge-issues?from=X&to=Y
func (s *server) handleEdgeIssues(w http.ResponseWriter, r *http.Request, id string) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from == "" || to == "" {
		http.Error(w, "from and to are required", 400)
		return
	}
	items, err := s.db.EdgeIssues(id, from, to)
	if err != nil {
		s.logger().Error("request failed", "path", r.URL.Path, "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, items)
}

// GET /api/snapshots/{id}/unassoc-epics
func (s *server) handleUnassocEpics(w http.ResponseWriter, id string) {
	rows, err := s.db.UnassocEpics(id, 500)
	if err != nil {
		s.logger().Error("request failed", "err", err)
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, rows)
}
