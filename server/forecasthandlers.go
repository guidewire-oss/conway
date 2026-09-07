package main

import (
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"net/http"
	"slices"
	"time"
)

// specs/028-portfolio-forecasts.md:95: a read-only computation still needs
// current plan ownership and current persisted manager authorization.
func (s *server) handleForecast(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	roles, err := s.db.EvidenceActor(r.Context(), c.Sub, time.Now().Unix())
	if err != nil {
		http.Error(w, "Current manager access is required. Sign in again.", http.StatusForbidden)
		return
	}
	if c.GameID != "" || (!c.Has("manager") && !c.Has("admin")) || (!slices.Contains(roles, "manager") && !slices.Contains(roles, "admin")) || (p.Owner != c.Sub && !slices.Contains(roles, "admin")) {
		http.Error(w, "Manager access to this plan is required.", http.StatusForbidden)
		return
	}
	var settings planning.ForecastSettings
	if !decodeReviewBody(w, r, &settings) {
		return
	}
	inputs, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		http.Error(w, "Could not read saved planning inputs: "+err.Error(), http.StatusBadRequest)
		return
	}
	result, err := planning.ComputeForecast(inputs, settings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, result)
}
